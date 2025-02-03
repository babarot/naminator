package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
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

	Clean bool `short:"c" long:"clean" description:"Clean up directories after renaming" required:"false"`

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

	images, err := getImages(args)
	if err != nil {
		return err
	}
	p := tea.NewProgram(newModel(len(images)))

	var errs error
	rename := func(photo Photo) error {
		var newPath string
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
		newPath = filepath.Join(dest, fmt.Sprintf("%s.%s",
			photo.CreatedAt.Format("2006-01-02_15-04-05"),
			photo.Extension,
		))
		if opt.Dryrun {
			p.Send(renameResultMsg{photo: photo, newPath: newPath, dryrun: true})
			return nil
		}
		p.Send(renameResultMsg{photo: photo, newPath: newPath})
		_ = os.MkdirAll(dest, 0755)
		if err := os.Rename(photo.Path, newPath); err != nil {
			// Use hashicorp/go-multierror instead of errors.Join (as of Go 1.20)
			// because this one is pretty good in output format.
			errs = multierror.Append(
				errs,
				fmt.Errorf("%s: failed to rename: %w", photo.Path, err))
		}
		return nil
	}

	ch := make(chan Photo, len(images))
	eg, ctx := errgroup.WithContext(ctx)
	for _, image := range images {
		image := image
		eg.Go(func() error {
			start := time.Now()
			photo, err := analyzeExifdata(filepath.Dir(image), image)
			select {
			case ch <- photo:
				p.Send(analyzeResultMsg{
					photo:    photo,
					duration: duration(time.Since(start)),
					err:      err,
				})
				_ = rename(photo)
			case <-ctx.Done():
				return ctx.Err()
			}
			return nil
		})
	}

	var photos []Photo

	// done := make(chan bool)
	go func() {
		_ = eg.Wait()
		close(ch)
		for photo := range ch {
			photos = append(photos, photo)
		}
		p.Send(finishMsg{})
		// done <- true
	}()

	if _, err := p.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
	// <-done

	if opt.Clean {
		for _, arg := range args {
			empty, err := isEmptyDir(arg)
			if err != nil {
				errs = multierror.Append(errs, err)
				continue
			}
			if opt.Dryrun {
				// TODO:
				// fmt.Printf("[INFO] (dryrun) Would remove %q if empty\n", arg)
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

func analyzeExifdata(dir, file string) (Photo, error) {
	et, err := exiftool.NewExiftool()
	if err != nil {
		return Photo{}, fmt.Errorf("failed to run exiftool: %w", err)
	}
	defer et.Close()

	fileInfos := et.ExtractMetadata(file)
	if len(fileInfos) == 0 {
		return Photo{Name: file}, errors.New("failed to extract metadata")
	}
	// ExtractMetadata can deal with multiple files at once but this function only uses one argument
	// so it's enough to reference the first element in fileInfos.
	fileInfo := fileInfos[0]

	if fileInfo.Err != nil {
		return Photo{Name: file}, fmt.Errorf("file info error: %w", err)
	}

	filename, err := fileInfo.GetString("FileName")
	if err != nil {
		return Photo{Name: filename}, fmt.Errorf("error on 'FileName': %w", err)
	}

	dateTime, err := fileInfo.GetString("SubSecDateTimeOriginal") // Use it instead of DateTimeOriginal
	if err != nil {
		return Photo{Name: filename}, fmt.Errorf("error on 'SubSecDateTimeOriginal': %w", err)
	}

	sourceFile, err := fileInfo.GetString("SourceFile")
	if err != nil {
		return Photo{Name: filename}, fmt.Errorf("error on 'SourceFile': %w", err)
	}

	ext, err := fileInfo.GetString("FileTypeExtension")
	if err != nil {
		return Photo{Name: filename}, fmt.Errorf("error on 'FileTypeExtension': %w", err)
	}

	createdAt, err := time.Parse("2006:01:02 15:04:05.000-07:00", dateTime)
	if err != nil {
		return Photo{Name: filename}, fmt.Errorf("failed to parse createdAt: %w", err)
	}

	// TODO:
	// return Photo{Name: filename}, fmt.Errorf("failed to parse createdAt: %w", errors.New("FAIL"))

	return Photo{
		Name:      filename,
		Path:      sourceFile,
		Dir:       filepath.Dir(dir), // dir of given path (dir).
		Extension: ext,
		CreatedAt: createdAt,
	}, nil
}

func getExt(file string) string {
	ext := filepath.Ext(file)
	switch ext {
	case ".ARW":
		return "raw"
	case ".HIF":
		return "heif"
	default:
		return strings.TrimPrefix(strings.ToLower(ext), ".") // remove dot
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
	dryrunStyle   = dotStyle
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#c53b53"))
	appStyle      = lipgloss.NewStyle().Margin(1, 2, 0, 2)
)

type duration time.Duration

func (d duration) String() string {
	sec := float64(d) / float64(time.Second)
	return fmt.Sprintf("%.2fs", sec)
}

type analyzeResultMsg struct {
	photo    Photo
	duration duration
	err      error
}

type renameResultMsg struct {
	photo   Photo
	newPath string
	dryrun  bool
	err     error
}

func (r analyzeResultMsg) Err() error { return r.err }
func (r renameResultMsg) Err() error  { return r.err }

func (r renameResultMsg) String() string {
	if r.err != nil {
		return fmt.Sprintf("%s %s: %v",
			errorStyle.Render("Error!"),
			r.photo.Name,
			r.err.Error())
	}
	if r.dryrun {
		return fmt.Sprintf("%s Would rename %s (%s)",
			dryrunStyle.Render("(dryrun)"),
			r.photo.Name,
			filepath.Base(r.newPath),
		)
	}
	return fmt.Sprintf("Renaming %s -> %s", r.photo.Name, filepath.Base(r.newPath))
}

func (r analyzeResultMsg) String() string {
	if r.err != nil {
		return fmt.Sprintf("%s %s: %v",
			errorStyle.Render("Error!"),
			r.photo.Name,
			r.err.Error())
	}
	return fmt.Sprintf("Analyzing exif %s %s",
		r.photo.Name,
		durationStyle.Render(r.duration.String()))
}

type resultMsg interface {
	String() string
	Err() error
}

type model struct {
	spinner        spinner.Model
	errors         []resultMsg
	results        []resultMsg
	quitting       bool
	renamed, total int
}

const numLastResults = 15

func newModel(total int) model {
	s := spinner.New()
	s.Style = spinnerStyle
	return model{
		spinner: s,
		results: make([]resultMsg, numLastResults),
		total:   total,
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
		if msg.Err() != nil {
			m.errors = append(m.errors, msg)
		}
		if len(m.errors) > 0 {
			var met bool
			var results []resultMsg
			for _, result := range m.results {
				if (result == nil || result.Err() == nil) && !met {
					met = true
				} else {
					results = append(results, result)
				}
			}
			if n := len(results); n >= numLastResults {
				m.results = append(results[n-numLastResults+1:], msg)
			} else {
				m.results = append(results, msg)
			}
		} else {
			m.results = append(m.results[1:], msg)
		}
		if _, ok := msg.(renameResultMsg); ok {
			m.renamed++
		}
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
		s += "That’s all for renaming!"
	} else {
		s += m.spinner.View() + " Processing photos..."
	}
	s += fmt.Sprintf(" (%d/%d)", m.renamed, m.total)

	s += "\n\n"

	for _, res := range m.results {
		switch res.(type) {
		case analyzeResultMsg, renameResultMsg:
			s += res.String()
		default:
			s += dotStyle.Render(strings.Repeat(".", 30))
		}
		s += "\n"
	}

	s += "\n"

	return appStyle.Render(s)
}
