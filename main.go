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
	defer logFile.Close()

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
	if opt.GroupExtFirst && !(opt.GroupByDate && opt.GroupByExt) {
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
		defer e.Close()
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

// processAll processes c.images with one worker per extractor in c.exifs
// and returns when all of them are done.
func (c CLI) processAll() {
	images := make(chan string)
	var wg sync.WaitGroup
	for _, exif := range c.exifs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for image := range images {
				c.process(exif, image)
			}
		}()
	}
	for _, image := range c.images {
		images <- image
	}
	close(images)
	wg.Wait()
}

func (c CLI) process(exif ExifExtractor, image string) {
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
		return
	}
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
		return photo, true, nil
	}
	_ = c.fs.MkdirAll(filepath.Dir(photo.RenamedPath), 0755)
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
		empty, err := isEmptyDir(path)
		if err != nil {
			c.sender.Send(cleanResultMsg{dir: base, err: fmt.Errorf("isEmptyDir: %w", err)})
			continue
		}
		if c.opt.Dryrun {
			c.sender.Send(cleanResultMsg{dir: base, dryrun: true})
			continue
		}
		if !empty {
			c.sender.Send(cleanResultMsg{dir: base, empty: false})
			continue
		}
		if err := c.fs.RemoveAll(path); err != nil {
			c.sender.Send(cleanResultMsg{dir: base, empty: true, err: err})
		} else {
			c.sender.Send(cleanResultMsg{dir: base, empty: true})
		}
	}
}

type Photo struct {
	Name        string
	Path        string
	RenamedPath string
	Dir         string
	Extension   string
	CreatedAt   time.Time
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
	jst := time.FixedZone("JST", 9*60*60)
	createdAt, err := getExifTime(fileInfo, "SubSecDateTimeOriginal", jst)
	if err != nil {
		var fallbackErr error
		createdAt, fallbackErr = getExifTime(fileInfo, "DateTimeOriginal", jst)
		if fallbackErr != nil {
			return photo, fmt.Errorf("error on both SubSecDateTimeOriginal (%v) and DateTimeOriginal: %w", err, fallbackErr)
		}
	}

	// Convert to JST if not already
	createdAt = createdAt.In(jst)

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

func getExifTime(fileInfo exiftool.FileMetadata, key string, loc *time.Location) (time.Time, error) {
	value, err := fileInfo.GetString(key)
	if err != nil {
		return time.Time{}, err
	}
	return parseExifTime(value, loc)
}

// parseExifTime parses an EXIF date time such as "2024:01:02 15:04:05",
// optionally followed by fractional seconds of any length and a time zone
// offset. A value without an offset is taken to be in loc.
func parseExifTime(value string, loc *time.Location) (time.Time, error) {
	// When parsing, Go accepts fractional seconds right after the seconds
	// field even if the layout does not have them.
	t, err := time.ParseInLocation("2006:01:02 15:04:05Z07:00", value, loc)
	if err == nil {
		return t, nil
	}
	return time.ParseInLocation("2006:01:02 15:04:05", value, loc)
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
			file := file
			mime, _ := mimetype.DetectFile(file)
			if !strings.Contains(mime.String(), "image") {
				continue
			}
			images = append(images, file)
		}
	}
	return images, nil
}

func isEmptyDir(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1) // Or f.Readdir(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err // Either not empty or error, suits both cases
}
