package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/barasher/go-exiftool"
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
	Interactive   bool       `short:"i" long:"interactive" description:"Preview the new paths and choose the format before renaming"`
	Format        string     `short:"f" long:"format" description:"The path to rename photos to, as {ext}/{date}/{date}_{time}"`
	Preset        string     `short:"p" long:"preset" description:"The format of a preset, by name, as \"by extension, then date\""`
	DestDir       string     `short:"d" long:"dest-dir" description:"The directory path where renamed photos will be moved" default:""`
	Dryrun        bool       `short:"n" long:"dry-run" description:"Simulate the command's actions without executing them"`
	Clean         bool       `short:"c" long:"clean" description:"Remove empty directories after renaming"`
	GroupByDate   bool       `short:"t" long:"group-by-date" description:"Deprecated: use --preset \"by date\""`
	GroupByExt    bool       `short:"e" long:"group-by-ext" description:"Deprecated: use --preset \"by extension\""`
	GroupExtFirst bool       `short:"E" long:"group-ext-first" description:"Deprecated: with -t and -e, use --preset \"by extension, then date\""`
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
	ReadDir(name string) ([]os.DirEntry, error)
}

// osFS is the real filesystem implementation.
type osFS struct{}

func (osFS) Rename(oldpath, newpath string) error         { return os.Rename(oldpath, newpath) }
func (osFS) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }
func (osFS) RemoveAll(path string) error                  { return os.RemoveAll(path) }
func (osFS) Stat(name string) (os.FileInfo, error)        { return os.Stat(name) }
func (osFS) ReadDir(name string) ([]os.DirEntry, error)   { return os.ReadDir(name) }

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
	exifs []ExifExtractor
	fs    FileSystem
	paths *pathReserver
	// moves records the renames dry-run would do, for clean to predict
	moves *pendingMoves
	// namer gives each photo its path; nil names photos by the flags
	// (legacyNamer)
	namer *namer
	// base and into are where rename targets are shown from (phaseStartMsg)
	base string
	into bool
	// canceled is closed when the user stops the run (Ctrl+C)
	canceled <-chan struct{}
	cancel   func()
}

// errCanceled is returned when the user stopped the run.
var errCanceled = errors.New("canceled")

func main() {
	if err := runMain(); err != nil {
		if errors.Is(err, errCanceled) {
			os.Exit(130)
		}
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

	cfgPath, err := configPath()
	if err != nil {
		return err
	}
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		return err
	}
	var namer *namer
	var note string
	if !opt.Interactive {
		namer, note, err = chooseNamer(opt, cfg, args)
		if err != nil {
			return err
		}
	} else if err := checkInteractive(opt); err != nil {
		return err
	}
	if note != "" {
		fmt.Fprintf(os.Stderr, "%s: %s\n", appName, note)
	}
	opt.Clean = opt.Clean || cfg.Clean

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

	base, into := displayBase(args, opt)
	cli := CLI{
		args: args,
		opt:  opt,
		logger: slog.New(slog.NewJSONHandler(
			logFile,
			&slog.HandlerOptions{Level: slog.LevelDebug}),
		),
		images: images,
		exifs:  exifs,
		fs:     osFS{},
		paths:  newPathReserver(),
		moves:  newPendingMoves(),
		namer:  namer,
		base:   base,
		into:   into,
	}
	first := phaseStartMsg{kind: phaseExif, keys: images, limit: len(exifs)}

	if !isatty.IsTerminal(os.Stdout.Fd()) {
		if opt.Interactive {
			return errors.New("-i needs a terminal")
		}
		disableStyles()
		cli.sender = newPlainSender(os.Stdout, first)
		return cli.run()
	}

	canceled := make(chan struct{})
	var once sync.Once
	cancel := func() { once.Do(func() { close(canceled) }) }
	cli.canceled = canceled
	cli.cancel = cancel

	if opt.Interactive {
		format, err := startFormat(opt, cfg, args)
		if err != nil {
			return err
		}
		tui := newTUIModel(args, images, format, opt.DestDir, opt.Dryrun, opt.Clean, cfg.allPresets(), cli.fs, cfgPath, cancel)
		return cli.runInteractive(tui)
	}

	model := newModel(first, cancel)
	if title := model.startCompact(); title != "" {
		fmt.Println(title)
	}
	cli.p = tea.NewProgram(model, tea.WithoutSignalHandler())
	cli.sender = cli.p
	return cli.run()
}

type duration time.Duration

func (d duration) String() string {
	sec := float64(d) / float64(time.Second)
	return fmt.Sprintf("%.2fs", sec)
}

// displayBase returns where rename targets are shown from: the destination
// directory, or else the directory given when it is the only argument.
func displayBase(args []string, opt Option) (base string, into bool) {
	if opt.DestDir != "" {
		return opt.DestDir, true
	}
	if len(args) == 1 {
		if fi, err := os.Stat(args[0]); err == nil && fi.IsDir() {
			return args[0], false
		}
	}
	return "", false
}

func (c CLI) run() error {
	return c.runWork(func() {
		// Process all photos, then remove empty directories
		c.processAll()
		c.finish()
	})
}

// finish removes the empty directories when asked, and ends the display.
func (c CLI) finish() {
	if c.opt.Clean && !c.isCanceled() {
		c.sender.Send(phaseStartMsg{kind: phaseClean, dry: c.opt.Dryrun})
		c.clean(c.args)
	}
	// Signal completion to stop UI rendering
	c.sender.Send(finishMsg{})
}

// runWork runs work with the display: plain without a terminal (c.p is
// nil), else the program c.p, which quits once work has sent finishMsg{}
// and everything is printed.
func (c CLI) runWork(work func()) error {
	c.logger.Debug("start")
	defer c.logger.Debug("end")

	if c.p == nil {
		work()
		return nil
	}
	if _, err := c.runProgram(c.p, work); err != nil {
		return err
	}
	if c.isCanceled() {
		return errCanceled
	}
	return nil
}

// runProgram runs p while work runs, and returns p's final model once both
// have ended. A signal reaches p as interruptMsg, as Ctrl+C does; a failure
// of p stops the work.
func (c CLI) runProgram(p *tea.Program, work func()) (tea.Model, error) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		work()
	}()
	// A signal stops the run as Ctrl+C does. After the first, the default
	// handling is back, so a second one ends the process at once.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	go func() {
		select {
		case <-sig:
			signal.Stop(sig)
			p.Send(interruptMsg{})
		case <-done:
		}
	}()
	final, err := p.Run()
	if err != nil {
		// The program failed: stop the work too.
		c.cancel()
	}
	// Wait for the work, so that no rename starts after this and the
	// exiftool processes are closed only once nothing uses them. Sends to
	// the ended program return at once.
	<-done
	// FIXME(bubbletea): drain the terminal's replies to the queries
	// bubbletea v2 sends, which it fails to read during shutdown; they
	// would leak into the shell. Only a terminal replies: draining
	// anything else (</dev/null, a pipe) would wait forever on Linux.
	if isatty.IsTerminal(os.Stdin.Fd()) {
		drainStdinAfterBubbletea()
	}
	return final, err
}

// isCanceled reports whether the user stopped the run.
func (c CLI) isCanceled() bool {
	select {
	case <-c.canceled:
		return true
	default:
		return false
	}
}

// processAll processes c.images and returns when all of them are done. It
// reads EXIF data with one worker per extractor in c.exifs, then renames the
// photos in the order they were taken.
func (c CLI) processAll() {
	photos := c.extractAll()
	if c.isCanceled() {
		return
	}
	c.renameAll(photos, true)
}

// sortByTime sorts photos in the order they were taken, the order they are
// renamed in: photos taken in the same second get _1, _2, ... in that
// order, and the files of one shot (e.g. RAW and HEIF, with the same
// original name) get the same number.
func sortByTime(photos []Photo) {
	slices.SortStableFunc(photos, func(a, b Photo) int {
		if n := a.CreatedAt.Compare(b.CreatedAt); n != 0 {
			return n
		}
		if n := strings.Compare(a.Name, b.Name); n != 0 {
			return n
		}
		return strings.Compare(a.Path, b.Path)
	})
}

// pathsOf returns the paths of photos, the rows of the rename phase.
func pathsOf(photos []Photo) []string {
	keys := make([]string, len(photos))
	for i, photo := range photos {
		keys[i] = photo.Path
	}
	return keys
}

// renameAll renames photos with their sidecars, in the order they were
// taken. announce starts the rename phase of the display, unless it was
// made with it.
func (c CLI) renameAll(photos []Photo, announce bool) {
	sortByTime(photos)
	sidecars, warnings := findSidecars(c.fs, photos)
	for i, photo := range photos {
		photos[i].Sidecars = sidecars[photo.Path]
	}
	if announce {
		c.sender.Send(phaseStartMsg{kind: phaseRename, keys: pathsOf(photos), base: c.base, into: c.into, dry: c.opt.Dryrun})
	}
	for _, w := range warnings {
		c.sender.Send(warnMsg{name: filepath.Base(w.path), text: w.reason})
		c.logger.Warn("sidecar not moved", "path", w.path, "reason", w.reason)
	}
	for _, photo := range photos {
		// Stop between renames: the one in hand is done whole
		if c.isCanceled() {
			return
		}
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
		wg.Go(func() {
			for image := range images {
				if photo, ok := c.extract(exif, image); ok {
					mu.Lock()
					photos = append(photos, photo)
					mu.Unlock()
				}
			}
		})
	}
feed:
	for _, image := range c.images {
		select {
		case images <- image:
		case <-c.canceled:
			break feed
		}
	}
	close(images)
	wg.Wait()
	return photos
}

func (c CLI) extract(exif ExifExtractor, image string) (Photo, bool) {
	c.sender.Send(exifStartMsg{image: image})
	startTime := time.Now()
	photo, err := exif.Extract(image)
	c.sender.Send(exifResultMsg{
		image:    image,
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
	c.sender.Send(renameResultMsg{photo: photo, dryrun: dryrun, err: err})
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

// buildNewPath computes the destination path for a photo based on options,
// as the flags -t, -e, -E and -d name photos (see legacyNamer).
func buildNewPath(photo Photo, opt Option) string {
	return legacyNamer(opt).target(photo)
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

// claim reserves path for src when it is free: not handed out yet, and no
// file other than src is there.
func (r *pathReserver) claim(fsys FileSystem, src, path string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.paths[path] || isOtherFile(fsys, path, src) {
		return false
	}
	r.paths[path] = true
	return true
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

// target is the path photo is to be renamed to, before it is reserved.
func (c CLI) target(photo Photo) string {
	if c.namer == nil {
		return buildNewPath(photo, c.opt)
	}
	return c.namer.target(photo)
}

func (c CLI) rename(photo Photo) (Photo, bool, error) {
	photo.RenamedPath = c.paths.reserve(c.fs, photo.Path, c.target(photo))
	// The sidecars go with the photo, named after its new name. A sidecar
	// whose path is taken stops the photo too, so that they keep matching.
	var moves [][2]string
	if photo.RenamedPath != photo.Path {
		for _, sc := range photo.Sidecars {
			to := sidecarTarget(photo.Path, photo.RenamedPath, sc)
			// DSC05501.xmp and DSC05501.XMP would be one file where case
			// does not matter, and the second would overwrite the first
			for _, m := range moves {
				if strings.EqualFold(m[1], to) {
					return photo, c.opt.Dryrun, fmt.Errorf("not moved: its sidecars %s and %s would go to the same path %s", filepath.Base(m[0]), filepath.Base(sc), to)
				}
			}
			if !c.paths.claim(c.fs, sc, to) {
				return photo, c.opt.Dryrun, fmt.Errorf("not moved: its sidecar %s would go to %s, which is taken", filepath.Base(sc), to)
			}
			moves = append(moves, [2]string{sc, to})
		}
	}
	for _, m := range moves {
		photo.RenamedSidecars = append(photo.RenamedSidecars, m[1])
	}
	if c.opt.Dryrun {
		c.moves.add(photo.Path, photo.RenamedPath)
		for _, m := range moves {
			c.moves.add(m[0], m[1])
		}
		return photo, true, nil
	}
	if err := c.fs.MkdirAll(filepath.Dir(photo.RenamedPath), 0755); err != nil {
		return photo, false, err
	}
	if err := c.fs.Rename(photo.Path, photo.RenamedPath); err != nil {
		return photo, false, err
	}
	for _, m := range moves {
		// Look again just before: a rename replaces a file that is there
		if isOtherFile(c.fs, m[1], m[0]) {
			return photo, false, fmt.Errorf("renamed, but its sidecar %s was not: %s is taken", filepath.Base(m[0]), m[1])
		}
		if err := c.fs.Rename(m[0], m[1]); err != nil {
			return photo, false, fmt.Errorf("renamed, but its sidecar %s was not: %w", filepath.Base(m[0]), err)
		}
	}
	return photo, false, nil
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
	// Camera is the model of the camera, from EXIF (empty if unknown)
	Camera string
	// Sidecars are the files that go with the photo (see findSidecars), and
	// RenamedSidecars where they went
	Sidecars        []string
	RenamedSidecars []string
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
			return photo, fmt.Errorf("error on both SubSecDateTimeOriginal (%w) and DateTimeOriginal: %w", err, fallbackErr)
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

	// The model is optional: only {camera} uses it
	camera, _ := fileInfo.GetString("Model")

	return Photo{
		Name:      filename,
		Path:      sourceFile,
		Dir:       filepath.Dir(path),
		Extension: ext,
		Camera:    strings.TrimSpace(camera),
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
