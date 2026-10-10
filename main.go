package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/barasher/go-exiftool"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gabriel-vasile/mimetype"
	"github.com/jessevdk/go-flags"
	"github.com/mattn/go-isatty"
	"github.com/nxadm/tail"
)

const appName = "naminator"

var (
	version  = "develop"
	revision = "HEAD"
)

// Option struct with the new GroupExtFirst flag
type Option struct {
	DestDir       string     `short:"d" long:"dest-dir" description:"The directory path where renamed photos will be moved" default:""`
	Dryrun        bool       `short:"n" long:"dry-run" description:"Simulate the command's actions without executing them"`
	GroupByDate   bool       `short:"t" long:"group-by-date" description:"Create a directory for each date and organize photos accordingly"`
	GroupByExt    bool       `short:"e" long:"group-by-ext" description:"Create a directory for each file extension and organize the photos accordingly"`
	GroupExtFirst bool       `short:"E" long:"group-ext-first" description:"Prioritize grouping by extension over date (requires -e and -t)"`
	Clean         bool       `short:"c" long:"clean" description:"Remove empty directories after renaming"`
	Meta          MetaOption `group:"Meta Options"`
}

type MetaOption struct {
	Debug   string `long:"debug" description:"View debug logs (default: \"full\")" optional-value:"full" optional:"yes" choice:"full" choice:"live"`
	Version bool   `short:"v" long:"version" description:"Show version"`
}

// Sender abstracts message sending to the UI.
type Sender interface {
	Send(msg tea.Msg)
}

// ExifExtractor extracts EXIF metadata from a file.
type ExifExtractor interface {
	Extract(path string) (Photo, error)
}

// FileSystem abstracts file system operations for testability.
type FileSystem interface {
	Rename(oldpath, newpath string) error
	MkdirAll(path string, perm os.FileMode) error
	RemoveAll(path string) error
	Stat(name string) (os.FileInfo, error)
}

// osFS is the real filesystem implementation.
type osFS struct{}

func (osFS) Rename(oldpath, newpath string) error          { return os.Rename(oldpath, newpath) }
func (osFS) MkdirAll(path string, perm os.FileMode) error  { return os.MkdirAll(path, perm) }
func (osFS) RemoveAll(path string) error                   { return os.RemoveAll(path) }
func (osFS) Stat(name string) (os.FileInfo, error)         { return os.Stat(name) }

// exiftoolExtractor is the real ExifExtractor using go-exiftool. It keeps
// one exiftool process running (stay_open) and reuses it for every file.
type exiftoolExtractor struct {
	et *exiftool.Exiftool
}

func newExiftoolExtractor() (exiftoolExtractor, error) {
	et, err := exiftool.NewExiftool()
	if err != nil {
		return exiftoolExtractor{}, fmt.Errorf("failed to run exiftool: %w", err)
	}
	return exiftoolExtractor{et: et}, nil
}

func (e exiftoolExtractor) Extract(path string) (Photo, error) {
	return getExifdata(e.et, path)
}

func (e exiftoolExtractor) Close() error {
	return e.et.Close()
}

type CLI struct {
	args   []string
	opt    Option
	logger *slog.Logger
	p      *tea.Program
	sender Sender
	images []string
	// exifs has one extractor per worker; images are processed in parallel
	// by len(exifs) workers.
	exifs  []ExifExtractor
	fs     FileSystem
	paths  *pathReserver
	// moves records the renames dry-run would do, for clean to predict
	moves *pendingMoves
}

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", appName, err)
		os.Exit(1)
	}
}

func runMain() error {
	var opt Option
	parser := flags.NewParser(&opt, flags.Default)
	parser.Name = appName
	parser.Usage = "[OPTIONS] [files... | dirs...]"
	args, err := parser.Parse()
	if err != nil {
		if flags.WroteHelp(err) {
			return nil
		}
		return err
	}

	if opt.Meta.Version {
		fmt.Printf("%s %s (%s)\n", appName, version, revision)
		return nil
	}

	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dataDir = filepath.Join(homeDir, ".local", "share")
	}
	logPath := filepath.Join(dataDir, "naminator", "debug.log")

	logDir := filepath.Dir(logPath)
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		err := os.MkdirAll(logDir, 0755)
		if err != nil {
			return err
		}
	}

	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer func() { _ = logFile.Close() }()

	if debug := opt.Meta.Debug; debug != "" {
		shouldFollow := isatty.IsTerminal(os.Stdout.Fd())
		tailConfig := tail.Config{
			ReOpen: shouldFollow,
			Follow: shouldFollow,
			Poll:   true,
			Logger: tail.DiscardingLogger,
		}
		switch debug {
		case "live": // like, "follow", "stream", "new"
			tailConfig.Location = &tail.SeekInfo{
				Offset: 0,
				Whence: io.SeekEnd,
			}
		case "full": // like, "all", "initial"
		default:
			return fmt.Errorf("%s: not supported debug type", debug)
		}
		t, err := tail.TailFile(logPath, tailConfig)
		if err != nil {
			return err
		}
		for line := range t.Lines {
			fmt.Println(line.Text)
		}
		return nil
	}

	// Validate the GroupExtFirst flag - it requires both GroupByDate and GroupByExt
	if opt.GroupExtFirst && (!opt.GroupByDate || !opt.GroupByExt) {
		return fmt.Errorf("--group-ext-first requires both --group-by-date and --group-by-ext")
	}

	if len(args) == 0 {
		return fmt.Errorf("too few arguments")
	}

	images, err := getImages(args)
	if err != nil {
		return err
	}

	if len(images) == 0 {
		return errors.New("no images given")
	}

	// go-exiftool serializes calls on one process, so give each worker its own
	var exifs []ExifExtractor
	for range min(runtime.NumCPU(), len(images)) {
		e, err := newExiftoolExtractor()
		if err != nil {
			return err
		}
		defer func() { _ = e.Close() }()
		exifs = append(exifs, e)
	}

	p := tea.NewProgram(newModel(len(images)))
	cli := CLI{
		args: args,
		opt:  opt,
		logger: slog.New(slog.NewJSONHandler(
			logFile,
			&slog.HandlerOptions{Level: slog.LevelDebug}),
		),
		p:      p,
		sender: p,
		images: images,
		exifs:  exifs,
		fs:     osFS{},
		paths:  newPathReserver(),
		moves:  newPendingMoves(),
	}

	return cli.run()
}

type duration time.Duration

func (d duration) String() string {
	sec := float64(d) / float64(time.Second)
	return fmt.Sprintf("%.2fs", sec)
}

func (c CLI) run() error {
	c.logger.Debug("start")
	defer c.logger.Debug("end")

	go func() {
		// Process all photos, then remove empty directories
		c.processAll()
		c.clean(c.args)
		// Signal completion to stop UI rendering
		c.sender.Send(finishMsg{})
	}()

	// No need to wait for the goroutine explicitly, as c.p.Run() blocks
	// until it receives finishMsg{}. The cleanup and message sending
	// happen in a separate goroutine, ensuring that Run() eventually exits.
	if _, err := c.p.Run(); err != nil {
		return err
	}

	return nil
}

// processAll processes c.images and returns when all of them are done. It
// reads EXIF data with one worker per extractor in c.exifs, then renames the
// photos in the order they were taken.
func (c CLI) processAll() {
	photos := c.extractAll()

	// Rename in the order the photos were taken, so that photos taken in the
	// same second get _1, _2, ... in that order, and the files of one shot
	// (e.g. RAW and HEIF, with the same original name) get the same number
	slices.SortStableFunc(photos, func(a, b Photo) int {
		if n := a.CreatedAt.Compare(b.CreatedAt); n != 0 {
			return n
		}
		if n := strings.Compare(a.Name, b.Name); n != 0 {
			return n
		}
		return strings.Compare(a.Path, b.Path)
	})
	for _, photo := range photos {
		c.renameAndReport(photo)
	}
}

// extractAll reads EXIF data of c.images in parallel and returns the photos
// it could read.
func (c CLI) extractAll() []Photo {
	images := make(chan string)
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		photos []Photo
	)
	for _, exif := range c.exifs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for image := range images {
				if photo, ok := c.extract(exif, image); ok {
					mu.Lock()
					photos = append(photos, photo)
					mu.Unlock()
				}
			}
		}()
	}
	for _, image := range c.images {
		images <- image
	}
	close(images)
	wg.Wait()
	return photos
}

func (c CLI) extract(exif ExifExtractor, image string) (Photo, bool) {
	startTime := time.Now()
	photo, err := exif.Extract(image)
	c.sender.Send(exifResultMsg{
		photo:    photo,
		duration: duration(time.Since(startTime)),
		err:      err,
	})
	if err != nil {
		c.logger.Error("failed to get exif, so skip to rename", "err", err,
			"name", photo.Name,
			"path", photo.Path)
		return photo, false
	}
	return photo, true
}

func (c CLI) renameAndReport(photo Photo) {
	photo, dryrun, err := c.rename(photo)
	if dryrun {
		c.sender.Send(renameResultMsg{photo: photo, dryrun: true})
	} else {
		c.sender.Send(renameResultMsg{photo: photo, dryrun: false, err: err})
	}
	if err != nil {
		c.logger.Error("failed to rename", "err", err,
			"name", photo.Name,
			"from", photo.Path,
			"to", photo.RenamedPath)
	} else {
		c.logger.Debug("renamed",
			"name", photo.Name,
			"from", photo.Path,
			"to", photo.RenamedPath)
	}
}

// buildNewPath computes the destination path for a photo based on options.
// This is a pure function with no side effects.
func buildNewPath(photo Photo, opt Option) string {
	dest := opt.DestDir
	if dest == "" {
		dest = photo.Dir
		if opt.GroupByDate || opt.GroupByExt {
			dest = filepath.Dir(dest)
		}
	}

	if opt.GroupByExt && opt.GroupByDate {
		if opt.GroupExtFirst {
			dest = filepath.Join(dest, photo.Extension)
			dest = filepath.Join(dest, photo.CreatedAt.Format("2006-01-02"))
		} else {
			dest = filepath.Join(dest, photo.CreatedAt.Format("2006-01-02"))
			dest = filepath.Join(dest, photo.Extension)
		}
	} else if opt.GroupByDate {
		dest = filepath.Join(dest, photo.CreatedAt.Format("2006-01-02"))
	} else if opt.GroupByExt {
		dest = filepath.Join(dest, photo.Extension)
	}

	return filepath.Join(dest, fmt.Sprintf("%s.%s",
		photo.CreatedAt.Format("2006-01-02_15-04-05"),
		photo.Extension,
	))
}

// pathReserver hands out destination paths so that no two photos in a run,
// and no photo and an existing file, end up with the same path.
type pathReserver struct {
	mu    sync.Mutex
	paths map[string]bool
}

func newPathReserver() *pathReserver {
	return &pathReserver{paths: map[string]bool{}}
}

// reserve returns want if it is free, or else the first free path made by
// adding _1, _2, ... before the extension. A path that src itself already
// occupies counts as free, so renaming a photo to its current name is a no-op.
func (r *pathReserver) reserve(fsys FileSystem, src, want string) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	ext := filepath.Ext(want)
	base := strings.TrimSuffix(want, ext)
	for i := 0; ; i++ {
		path := want
		if i > 0 {
			path = fmt.Sprintf("%s_%d%s", base, i, ext)
		}
		if r.paths[path] || isOtherFile(fsys, path, src) {
			continue
		}
		r.paths[path] = true
		return path
	}
}

// isOtherFile reports whether path exists and is not the file at src.
func isOtherFile(fsys FileSystem, path, src string) bool {
	fi, err := fsys.Stat(path)
	if err != nil {
		return false
	}
	srcFi, err := fsys.Stat(src)
	return err != nil || !os.SameFile(fi, srcFi)
}

func (c CLI) rename(photo Photo) (Photo, bool, error) {
	photo.RenamedPath = c.paths.reserve(c.fs, photo.Path, buildNewPath(photo, c.opt))
	if c.opt.Dryrun {
		c.moves.add(photo.Path, photo.RenamedPath)
		return photo, true, nil
	}
	if err := c.fs.MkdirAll(filepath.Dir(photo.RenamedPath), 0755); err != nil {
		return photo, false, err
	}
	return photo, false, c.fs.Rename(photo.Path, photo.RenamedPath)
}

func (c CLI) clean(paths []string) {
	if !c.opt.Clean {
		return
	}
	for _, path := range paths {
		base := filepath.Base(path)
		fi, err := c.fs.Stat(path)
		if err != nil {
			// Skip silently if path doesn't exist (e.g., renamed individual files)
			continue
		}
		if !fi.IsDir() {
			continue
		}
		// In dry-run nothing has been moved, so predict with the pending moves
		var moves *pendingMoves
		if c.opt.Dryrun {
			moves = c.moves
		}
		removable, dirs, err := emptyDirs(path, moves)
		if err != nil {
			c.sender.Send(cleanResultMsg{dir: base, err: fmt.Errorf("emptyDirs: %w", err)})
			continue
		}
		for _, dir := range dirs {
			// Show a subdirectory as e.g. "DCIM/100MSDCF"
			name := base
			if rel, err := filepath.Rel(path, dir); err == nil && rel != "." {
				name = filepath.Join(base, rel)
			}
			if c.opt.Dryrun {
				c.sender.Send(cleanResultMsg{dir: name, dryrun: true, empty: true})
				continue
			}
			if err := c.fs.RemoveAll(dir); err != nil {
				c.sender.Send(cleanResultMsg{dir: name, empty: true, err: err})
			} else {
				c.sender.Send(cleanResultMsg{dir: name, empty: true})
			}
		}
		if !removable {
			c.sender.Send(cleanResultMsg{dir: base, dryrun: c.opt.Dryrun, empty: false})
		}
	}
}

type Photo struct {
	Name        string
	Path        string
	RenamedPath string
	Dir         string
	Extension   string
	// CreatedAt is the clock time where the photo was taken (see parseExifTime)
	CreatedAt time.Time
}

func getExifdata(et *exiftool.Exiftool, path string) (Photo, error) {
	base := filepath.Base(path)

	photo := Photo{
		Name: base,
		Path: path,
	}

	fileInfos := et.ExtractMetadata(path)
	if len(fileInfos) == 0 {
		return photo, errors.New("failed to extract metadata")
	}
	// ExtractMetadata can deal with multiple files at once but this function only uses one argument
	// so it's enough to reference the first element in fileInfos.
	fileInfo := fileInfos[0]

	if fileInfo.Err != nil {
		return photo, fmt.Errorf("file info error: %w", fileInfo.Err)
	}

	filename, err := fileInfo.GetString("FileName")
	photo.Name = filename
	if err != nil {
		return photo, fmt.Errorf("error on 'FileName': %w", err)
	}

	// Try SubSecDateTimeOriginal first, fallback to DateTimeOriginal if it is
	// missing or cannot be parsed
	createdAt, err := getExifTime(fileInfo, "SubSecDateTimeOriginal")
	if err != nil {
		var fallbackErr error
		createdAt, fallbackErr = getExifTime(fileInfo, "DateTimeOriginal")
		if fallbackErr != nil {
			return photo, fmt.Errorf("error on both SubSecDateTimeOriginal (%v) and DateTimeOriginal: %w", err, fallbackErr)
		}
	}

	sourceFile, err := fileInfo.GetString("SourceFile")
	if err != nil {
		return photo, fmt.Errorf("error on 'SourceFile': %w", err)
	}

	ext, err := fileInfo.GetString("FileTypeExtension")
	if err != nil {
		return photo, fmt.Errorf("error on 'FileTypeExtension': %w", err)
	}

	return Photo{
		Name:      filename,
		Path:      sourceFile,
		Dir:       filepath.Dir(path),
		Extension: ext,
		CreatedAt: createdAt,
	}, nil
}

func getExifTime(fileInfo exiftool.FileMetadata, key string) (time.Time, error) {
	value, err := fileInfo.GetString(key)
	if err != nil {
		return time.Time{}, err
	}
	return parseExifTime(value)
}

// parseExifTime parses an EXIF date time such as "2024:01:02 15:04:05",
// optionally followed by fractional seconds of any length and a time zone
// offset.
//
// The result keeps the clock time where the photo was taken, as EXIF records
// it; it is never converted to another time zone. A value with an offset keeps
// that offset, and one without is put in UTC only to hold the clock time as is,
// since a zone with daylight saving time would shift times that it skips.
func parseExifTime(value string) (time.Time, error) {
	// When parsing, Go accepts fractional seconds right after the seconds
	// field even if the layout does not have them.
	t, err := time.Parse("2006:01:02 15:04:05Z07:00", value)
	if err == nil {
		return t, nil
	}
	return time.Parse("2006:01:02 15:04:05", value)
}

func walkDir(root string) ([]string, error) {
	files := []string{}
	err := filepath.WalkDir(root, func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

func getImages(dirs []string) ([]string, error) {
	var images []string
	for _, dir := range dirs {
		files, err := walkDir(dir)
		if err != nil {
			return []string{}, err
		}
		for _, file := range files {
			mime, _ := mimetype.DetectFile(file)
			if !strings.Contains(mime.String(), "image") {
				continue
			}
			images = append(images, file)
		}
	}
	return images, nil
}

// junkFiles are files that the OS creates by itself. A directory that has
// only these left counts as empty.
var junkFiles = map[string]bool{
	".DS_Store":   true, // macOS Finder
	"Thumbs.db":   true, // Windows Explorer
	"desktop.ini": true, // Windows Explorer
}

// pendingMoves records the renames that dry-run would do but has not done.
type pendingMoves struct {
	mu   sync.Mutex
	from map[string]bool // files that would be moved
	to   []string        // paths they would be moved to
}

func newPendingMoves() *pendingMoves {
	return &pendingMoves{from: map[string]bool{}}
}

// add records a move from src to dst. It does nothing on a nil receiver.
func (m *pendingMoves) add(src, dst string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.from[absPath(src)] = true
	m.to = append(m.to, absPath(dst))
}

// movedAway reports whether the file at path would be moved.
func (m *pendingMoves) movedAway(path string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.from[absPath(path)]
}

// movedInto reports whether a file would be moved to somewhere under dir.
func (m *pendingMoves) movedInto(dir string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	prefix := absPath(dir) + string(filepath.Separator)
	for _, to := range m.to {
		if strings.HasPrefix(to, prefix) {
			return true
		}
	}
	return false
}

func absPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}

// emptyDirs reports whether dir is empty, and returns the empty directories
// to remove: dir itself if it is empty, or else the outermost empty
// directories under it. A directory is empty when it has nothing but junk
// files and empty directories.
//
// With moves, it predicts the result after those moves: files that would be
// moved away count as gone, and a directory that a file would be moved into
// is not empty.
func emptyDirs(dir string, moves *pendingMoves) (bool, []string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, nil, err
	}
	empty := true
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			subEmpty, subDirs, err := emptyDirs(filepath.Join(dir, entry.Name()), moves)
			if err != nil {
				return false, nil, err
			}
			empty = empty && subEmpty
			dirs = append(dirs, subDirs...)
			continue
		}
		if !junkFiles[entry.Name()] && !moves.movedAway(filepath.Join(dir, entry.Name())) {
			empty = false
		}
	}
	if empty && moves.movedInto(dir) {
		empty = false
	}
	if empty {
		return true, []string{dir}, nil
	}
	return false, dirs, nil
}
