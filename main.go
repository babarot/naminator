package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/barasher/go-exiftool"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gabriel-vasile/mimetype"
	"github.com/hashicorp/go-multierror"
	"github.com/jessevdk/go-flags"
	"golang.org/x/sync/errgroup"
)

const appName = "naminator"

var (
	version  = "develop"
	revision = "HEAD"
)

type Option struct {
	DestDir string `short:"d" long:"dest-dir" description:"Directory path to move renamed photos" required:"false" default:""`
	Dryrun  bool   `short:"n" long:"dry-run" description:"Displays the operations that would be performed using the specified command without actually running them" required:"false"`

	GroupByDate bool `short:"t" long:"group-by-date" description:"Create a date directory and classify the photos for each date" required:"false"`
	GroupByExt  bool `short:"e" long:"group-by-ext" description:"Create an extension directory and classify the photos for each ext" required:"false"`

	Clean     bool `short:"c" long:"clean" description:"Clean up directories after renaming" required:"false"`
	WithIndex bool `long:"with-index" description:"Include index in a file name" required:"false"`

	Version bool `short:"v" long:"version" description:"Show version"`
}

type Photo struct {
	Name      string
	Path      string
	Dir       string
	Extension string
	CreatedAt time.Time
}

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] error occured while processing %s: %v\n", appName, err)
		os.Exit(1)
	}
}

func runMain() error {
	ctx := context.Background()

	var opt Option
	// parser := flags.NewParser(&opt, flags.Default & ^flags.HelpFlag) // if remove default help flag
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

	if opt.Version {
		fmt.Printf("%s %s (%s)\n", appName, version, revision)
		return nil
	}

	if len(args) == 0 {
		return fmt.Errorf("too few arguments")
	}

	// var allPhotos []Photo
	images, err := getImages(args)
	if err != nil {
		return err
	}
	p := tea.NewProgram(newModel())

	ch := make(chan Photo, len(images))
	eg, ctx := errgroup.WithContext(ctx)
	for _, image := range images {
		// walkDir can traverse dirs or files
		image := image
		eg.Go(func() error {
			start := time.Now()
			photo, err := analyzeExifdata(filepath.Dir(image), image)
			if err != nil {
				log.Print(fmt.Errorf("%s: failed to get EXIF data: %w", image, err))
				return nil
			}
			select {
			case ch <- photo:
				p.Send(resultMsg{photo: photo, duration: duration(time.Since(start))})
			case <-ctx.Done():
				return ctx.Err()
			}
			return nil
		})
	}

	var photos []Photo
	// go func() {
	// 	// do not handle error at this time
	// 	// because it would be done at the end of this func
	// 	_ = eg.Wait()
	// 	close(ch)
	// 	for photo := range ch {
	// 		photos = append(photos, photo)
	// 	}
	// }()

	// handle error in goroutines (secondary wait)
	// return photos, eg.Wait()
	// 	allPhotos = append(allPhotos, photos...)
	// }

	// allPhotos = photos

	var errs error
	done := make(chan bool)
	go func() {
		_ = eg.Wait()
		close(ch)
		for photo := range ch {
			photos = append(photos, photo)
		}
		sort.Slice(photos, func(i, j int) bool {
			return photos[i].CreatedAt.Before(photos[j].CreatedAt)
		})
		var newPath string
		start := time.Now()
		// p.Send(resultMsg{photo: photo, duration: })
		for index, photo := range photos {
			dest := opt.DestDir
			if dest == "" {
				dest = photo.Dir
			}
			if opt.GroupByDate {
				dt := photo.CreatedAt.Format("2006-01-02")
				dest = filepath.Join(dest, dt)
			}
			if opt.GroupByExt {
				ext := getExt(photo.Path)
				dest = filepath.Join(dest, ext)
			}
			if opt.WithIndex {
				newPath = filepath.Join(dest, fmt.Sprintf("%s-%03d.%s",
					photo.CreatedAt.Format("2006-01-02_15-04-05"),
					index+1,
					photo.Extension,
				))
			} else {
				newPath = filepath.Join(dest, fmt.Sprintf("%s.%s",
					photo.CreatedAt.Format("2006-01-02_15-04-05"),
					photo.Extension,
				))
			}
			if opt.Dryrun {
				p.Send(resultMsg{photo: photo, duration: duration(time.Since(start)), mv: true})
				// fmt.Printf("[INFO] (dryrun): Renaming %q to %q\n", photo.Path, newPath)
				continue
			}
			// fmt.Printf("[INFO] Renaming %q to %q\n", photo.Path, newPath)
			p.Send(resultMsg{photo: photo, duration: duration(time.Since(start)), mv: true})
			_ = os.MkdirAll(dest, 0755)
			if err := os.Rename(photo.Path, newPath); err != nil {
				// Use hashicorp/go-multierror instead of errors.Join (as of Go 1.20)
				// because this one is pretty good in output format.
				errs = multierror.Append(
					errs,
					fmt.Errorf("%s: failed to rename: %w", photo.Path, err))
			}
		}
		p.Send(finishMsg{})
		done <- true
	}()

	if _, err := p.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
	<-done

	if opt.Clean {
		for _, arg := range args {
			empty, err := isEmptyDir(arg)
			if err != nil {
				errs = multierror.Append(errs, err)
				continue
			}
			if opt.Dryrun {
				fmt.Printf("[INFO] (dryrun) Would remove %q if empty\n", arg)
				continue
			}
			if !empty {
				fmt.Printf("[INFO] skip to clean dir %q because NOT empty\n", arg)
				continue
			}
			if err := os.RemoveAll(arg); err != nil {
				errs = multierror.Append(
					errs,
					fmt.Errorf("%s: failed to remove dir: %w", arg, err))
				continue
			}
			fmt.Printf("[INFO] Removed %q because empty\n", arg)
		}
	}

	return errs
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

func getPhotos(ctx context.Context, dir string, p *tea.Program) ([]Photo, error) {
	ch := make(chan Photo)
	eg, ctx := errgroup.WithContext(ctx)

	// walkDir can traverse dirs or files
	files, err := walkDir(dir)
	if err != nil {
		return []Photo{}, err
	}

	for _, file := range files {
		file := file
		mime, _ := mimetype.DetectFile(file)
		if !strings.Contains(mime.String(), "image") {
			continue
		}
		eg.Go(func() error {
			start := time.Now()
			photo, err := analyzeExifdata(dir, file)
			if err != nil {
				log.Print(fmt.Errorf("%s: failed to get EXIF data: %w", file, err))
				return nil
			}
			select {
			case ch <- photo:
				p.Send(resultMsg{photo: photo, duration: duration(time.Since(start))})
			case <-ctx.Done():
				return ctx.Err()
			}
			return nil
		})
	}

	go func() {
		// do not handle error at this time
		// because it would be done at the end of this func
		_ = eg.Wait()
		close(ch)
	}()

	var photos []Photo
	for photo := range ch {
		photos = append(photos, photo)
	}

	// handle error in goroutines (secondary wait)
	return photos, eg.Wait()
}

func analyzeExifdata(dir, file string) (Photo, error) {
	et, err := exiftool.NewExiftool()
	if err != nil {
		return Photo{}, fmt.Errorf("failed to run exiftool: %w", err)
	}
	defer et.Close()

	fileInfos := et.ExtractMetadata(file)
	if len(fileInfos) == 0 {
		return Photo{}, errors.New("failed to extract metadata")
	}
	// ExtractMetadata can deal with multiple files at once but this function only uses one argument
	// so it's enough to reference the first element in fileInfos.
	fileInfo := fileInfos[0]

	if fileInfo.Err != nil {
		return Photo{}, fmt.Errorf("file info error: %w", err)
	}

	filename, err := fileInfo.GetString("FileName")
	if err != nil {
		return Photo{}, fmt.Errorf("error on 'FileName': %w", err)
	}

	dateTime, err := fileInfo.GetString("SubSecDateTimeOriginal") // Use it instead of DateTimeOriginal
	if err != nil {
		return Photo{}, fmt.Errorf("error on 'SubSecDateTimeOriginal': %w", err)
	}

	sourceFile, err := fileInfo.GetString("SourceFile")
	if err != nil {
		return Photo{}, fmt.Errorf("error on 'SourceFile': %w", err)
	}

	ext, err := fileInfo.GetString("FileTypeExtension")
	if err != nil {
		return Photo{}, fmt.Errorf("error on 'FileTypeExtension': %w", err)
	}

	createdAt, err := time.Parse("2006:01:02 15:04:05.000-07:00", dateTime)
	if err != nil {
		return Photo{}, fmt.Errorf("failed to parse createdAt: %w", err)
	}

	return Photo{
		Name:      filename,
		Path:      sourceFile,
		Dir:       filepath.Dir(dir), // dir of given path (dir).
		Extension: ext,
		CreatedAt: createdAt,
	}, nil
}

func getExt(file string) string {
	mime, _ := mimetype.DetectFile(file)
	if !strings.Contains(mime.String(), "image") {
		log.Panicf("%s: mimetype is not image", mime.String())
	}
	ext := filepath.Ext(file)
	switch ext {
	case ".ARW":
		return "raw"
	case ".HIF":
		return "heif"
	default:
		return strings.ToLower(ext[1:]) // remove dot
	}
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

var (
	spinnerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Margin(1, 0)
	dotStyle      = helpStyle.UnsetMargins()
	durationStyle = dotStyle
	appStyle      = lipgloss.NewStyle().Margin(1, 2, 0, 2)
)

type duration time.Duration

func (d duration) String() string {
	sec := float64(d) / float64(time.Second)
	return fmt.Sprintf("%.2fs", sec)
}

type resultMsg struct {
	duration duration
	photo    Photo
	mv       bool
}

func (r resultMsg) String() string {
	if r.duration == 0 {
		return dotStyle.Render(strings.Repeat(".", 30))
	}
	if r.mv {
		return fmt.Sprintf("🍔 moving %s %s", r.photo.Name,
			durationStyle.Render(r.duration.String()))
	}
	return fmt.Sprintf("🍔 Checking %s %s", r.photo.Name,
		durationStyle.Render(r.duration.String()))
}

type model struct {
	spinner  spinner.Model
	results  []resultMsg
	quitting bool
}

func newModel() model {
	const numLastResults = 10
	s := spinner.New()
	s.Style = spinnerStyle
	return model{
		spinner: s,
		results: make([]resultMsg, numLastResults),
	}
}

func (m model) Init() tea.Cmd {
	return m.spinner.Tick
}

type finishMsg struct{}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case finishMsg:
		m.quitting = true
		return m, tea.Quit
	case resultMsg:
		m.results = append(m.results[1:], msg)
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

func (m model) View() string {
	var s string

	if m.quitting {
		s += "That’s all for today!"
	} else {
		s += m.spinner.View() + " Eating food..."
	}

	s += "\n\n"

	for _, res := range m.results {
		s += res.String() + "\n"
	}

	s += "\n"

	return appStyle.Render(s)
}
