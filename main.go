package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/babarot/naminator/internal/config"
	"github.com/babarot/naminator/internal/photo"
	"github.com/babarot/naminator/internal/rename"

	tea "charm.land/bubbletea/v2"
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
	Interactive   bool       `short:"i" long:"interactive" hidden:"yes" description:"Preview the new paths and choose the format before renaming (the default in a terminal)"`
	Format        string     `short:"f" long:"format" description:"The path to rename photos to, as {ext}/{date}/{date}_{time}"`
	Preset        string     `short:"p" long:"preset" description:"The format of a preset, by name, as \"by extension, then date\""`
	DestDir       string     `short:"d" long:"dest-dir" description:"The directory path where renamed photos will be moved" default:""`
	Dryrun        bool       `short:"n" long:"dry-run" description:"Simulate the command's actions without executing them"`
	Clean         bool       `short:"c" long:"clean" description:"Remove empty directories after renaming"`
	Yes           bool       `short:"y" long:"yes" description:"Rename without asking, also in a terminal"`
	GroupByDate   bool       `short:"t" long:"group-by-date" description:"Deprecated: use --preset \"by date\""`
	GroupByExt    bool       `short:"e" long:"group-by-ext" description:"Deprecated: use --preset \"by extension\""`
	GroupExtFirst bool       `short:"E" long:"group-ext-first" description:"Deprecated: with -t and -e, use --preset \"by extension, then date\""`
	Meta          MetaOption `group:"Meta Options"`
}

type MetaOption struct {
	Debug   string `long:"debug" description:"View debug logs (default: \"full\")" optional-value:"full" optional:"yes" choice:"full" choice:"live"`
	Version bool   `short:"v" long:"version" description:"Show version"`
}

// osFS is the real filesystem implementation.
type osFS struct{}

func (osFS) Rename(oldpath, newpath string) error         { return os.Rename(oldpath, newpath) }
func (osFS) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }
func (osFS) RemoveAll(path string) error                  { return os.RemoveAll(path) }
func (osFS) Stat(name string) (os.FileInfo, error)        { return os.Stat(name) }
func (osFS) ReadDir(name string) ([]os.DirEntry, error)   { return os.ReadDir(name) }

// Sender abstracts message sending to the UI.
type Sender interface {
	Send(msg tea.Msg)
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
	exifs []photo.Extractor
	fs    rename.FileSystem
	paths *rename.Reserver
	// moves records the renames dry-run would do, for clean to predict
	moves *rename.PendingMoves
	// namer gives each photo its path; nil names photos by the flags
	// (rename.NewLegacyNamer)
	namer *rename.Namer
	// planned are the paths the interactive screen showed, by photo path:
	// a photo goes there unless the path is taken by then
	planned map[string]string
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

	cfgPath, err := config.Path()
	if err != nil {
		return err
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	var namer *rename.Namer
	var note string
	// The screen needs a terminal both ways: with stdin from a file or a
	// pipe, the run is unattended and must not wait for a key
	opt.Interactive = interactive(opt, isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd()))
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

	images, err := photo.Images(args)
	if err != nil {
		return err
	}

	if len(images) == 0 {
		return errors.New("no images given")
	}

	// go-exiftool serializes calls on one process, so give each worker its own
	var exifs []photo.Extractor
	for range min(runtime.NumCPU(), len(images)) {
		e, err := photo.NewExiftool()
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
		paths:  rename.NewReserver(),
		moves:  rename.NewPendingMoves(),
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
		tui := newTUIModel(args, images, format, opt.DestDir, opt.Dryrun, opt.Clean, cfg.AllPresets(), cli.fs, cfgPath, cancel)
		return cli.runInteractive(tui)
	}

	model := newModel(first, cancel)
	model.awaitReply = true
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
	// A signal stops the run as Ctrl+C does, from before the work starts
	// until this returns: the work can end long before p, as the reading
	// under the screen does. It also cancels the run, so that a signal as
	// p ends starts no next program. After the first, the default handling
	// is back, so a second one ends the process at once.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	interrupted := func() {
		if c.cancel != nil {
			c.cancel()
		}
	}
	ran := make(chan struct{})
	forwarded := make(chan struct{})
	go func() {
		defer close(forwarded)
		select {
		case <-sig:
			signal.Stop(sig)
			interrupted()
			p.Send(interruptMsg{})
		case <-ran:
		}
	}()
	defer func() {
		close(ran)
		<-forwarded
		// a signal that came as p ended, before it was forwarded
		select {
		case <-sig:
			interrupted()
		default:
		}
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		work()
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

// renameAll renames photos with their sidecars, in the order they were
// taken. announce starts the rename phase of the display, unless it was
// made with it.
func (c CLI) renameAll(photos []photo.Photo, announce bool) {
	photo.SortByTime(photos)
	sidecars, warnings := rename.FindSidecars(c.fs, photos)
	for i, photo := range photos {
		photos[i].Sidecars = sidecars[photo.Path]
	}
	if announce {
		c.sender.Send(phaseStartMsg{kind: phaseRename, keys: photo.Paths(photos), base: c.base, into: c.into, dry: c.opt.Dryrun})
	}
	for _, w := range warnings {
		c.sender.Send(warnMsg{name: filepath.Base(w.Path), text: w.Reason})
		c.logger.Warn("sidecar not moved", "path", w.Path, "reason", w.Reason)
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
func (c CLI) extractAll() []photo.Photo {
	images := make(chan string)
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		photos []photo.Photo
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

func (c CLI) extract(exif photo.Extractor, image string) (photo.Photo, bool) {
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

func (c CLI) renameAndReport(photo photo.Photo) {
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
// as the flags -t, -e, -E and -d name photos (see rename.NewLegacyNamer).
func buildNewPath(photo photo.Photo, opt Option) string {
	return rename.NewLegacyNamer(opt.legacy()).Target(photo)
}

// target is the path photo is to be renamed to, before it is reserved.
func (c CLI) target(photo photo.Photo) string {
	if c.namer == nil {
		return buildNewPath(photo, c.opt)
	}
	return c.namer.Target(photo)
}

func (c CLI) rename(photo photo.Photo) (photo.Photo, bool, error) {
	// The path previewed, as shown: renaming one by one would free paths
	// the preview saw taken, and number photos otherwise. A path taken by
	// now is reserved as any other, so that nothing is overwritten.
	if to, ok := c.planned[photo.Path]; ok && c.paths.Claim(c.fs, photo.Path, to) {
		photo.RenamedPath = to
	} else {
		photo.RenamedPath = c.paths.Reserve(c.fs, photo.Path, c.target(photo))
	}
	// The sidecars go with the photo, named after its new name. A sidecar
	// whose path is taken stops the photo too, so that they keep matching.
	var moves [][2]string
	if photo.RenamedPath != photo.Path {
		for _, sc := range photo.Sidecars {
			to := rename.SidecarTarget(photo.Path, photo.RenamedPath, sc)
			// DSC05501.xmp and DSC05501.XMP would be one file where case
			// does not matter, and the second would overwrite the first
			for _, m := range moves {
				if strings.EqualFold(m[1], to) {
					return photo, c.opt.Dryrun, fmt.Errorf("not moved: its sidecars %s and %s would go to the same path %s", filepath.Base(m[0]), filepath.Base(sc), to)
				}
			}
			if !c.paths.Claim(c.fs, sc, to) {
				return photo, c.opt.Dryrun, fmt.Errorf("not moved: its sidecar %s would go to %s, which is taken", filepath.Base(sc), to)
			}
			moves = append(moves, [2]string{sc, to})
		}
	}
	for _, m := range moves {
		photo.RenamedSidecars = append(photo.RenamedSidecars, m[1])
	}
	if c.opt.Dryrun {
		c.moves.Add(photo.Path, photo.RenamedPath)
		for _, m := range moves {
			c.moves.Add(m[0], m[1])
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
		if rename.IsOtherFile(c.fs, m[1], m[0]) {
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
		var moves *rename.PendingMoves
		if c.opt.Dryrun {
			moves = c.moves
		}
		removable, dirs, err := rename.EmptyDirs(path, moves)
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
