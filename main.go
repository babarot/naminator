package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/barasher/go-exiftool"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gabriel-vasile/mimetype"
	"github.com/jessevdk/go-flags"
)

const appName = "naminator"

var (
	version  = "develop"
	revision = "HEAD"
)

type Option struct {
	DestDir     string `short:"d" long:"dest-dir" description:"The directory path where renamed photos will be moved" default:""`
	Dryrun      bool   `short:"n" long:"dry-run" description:"Simulates the command's actions without executing them"`
	GroupByDate bool   `short:"t" long:"group-by-date" description:"Create a directory for each date and organize photos accordingly"`
	GroupByExt  bool   `short:"e" long:"group-by-ext" description:"Create a directory for each file extension and organize the photos accordingly"`
	Clean       bool   `short:"c" long:"clean" description:"Remove empty directories after renaming."`
	Version     bool   `short:"v" long:"version" description:"Show version"`
}

type Photo struct {
	Name        string
	Path        string
	RenamedPath string
	Dir         string
	Extension   string
	CreatedAt   time.Time
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

	if len(images) == 0 {
		return errors.New("no images given")
	}

	p := tea.NewProgram(newModel(len(images)))

	rename := func(photo Photo) (Photo, bool, error) {
		var newPath string
		dest := opt.DestDir
		if dest == "" {
			dest = photo.Dir
		}
		baseDir := dest // keep dest at this point
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
		photo.RenamedPath = strings.TrimLeft(newPath, baseDir)
		// photo.RenamedPath = newPath
		if opt.Dryrun {
			return photo, true, nil
		}
		_ = os.MkdirAll(dest, 0755)
		return photo, false, os.Rename(photo.Path, newPath)
	}

	var wg sync.WaitGroup

	for _, image := range images {
		imagePath := image
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			photo, err := analyzeExifdata(imagePath)
			p.Send(analyzeResultMsg{
				photo:    photo,
				duration: duration(time.Since(start)),
				err:      err,
			})
			photo, dryrun, err := rename(photo)
			if dryrun {
				p.Send(renameResultMsg{photo: photo, dryrun: true})
			} else {
				p.Send(renameResultMsg{photo: photo, dryrun: false, err: err})
			}
		}()
	}

	// done := make(chan bool)
	go func() {
		wg.Wait()
		if opt.Clean {
			for _, arg := range args {
				empty, err := isEmptyDir(arg)
				if err != nil {
					p.Send(cleanResultMsg{err: err})
					continue
				}
				if opt.Dryrun {
					p.Send(cleanResultMsg{dir: arg, dryrun: true})
					continue
				}
				if !empty {
					p.Send(cleanResultMsg{dir: arg, empty: false})
					continue
				}
				if err := os.RemoveAll(arg); err != nil {
					p.Send(cleanResultMsg{dir: arg, empty: true, err: err})
				} else {
					p.Send(cleanResultMsg{dir: arg, empty: true})
				}
			}
		}
		p.Send(finishMsg{})
		// done <- true
	}()

	if _, err := p.Run(); err != nil {
		return err
	}
	// <-done

	return nil
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

func analyzeExifdata(path string) (Photo, error) {
	et, err := exiftool.NewExiftool()
	if err != nil {
		return Photo{}, fmt.Errorf("failed to run exiftool: %w", err)
	}
	defer et.Close()

	base := filepath.Base(path)
	dir := filepath.Dir(path)
	fileInfos := et.ExtractMetadata(path)
	if len(fileInfos) == 0 {
		return Photo{Name: base}, errors.New("failed to extract metadata")
	}
	// ExtractMetadata can deal with multiple files at once but this function only uses one argument
	// so it's enough to reference the first element in fileInfos.
	fileInfo := fileInfos[0]

	if fileInfo.Err != nil {
		return Photo{Name: base}, fmt.Errorf("file info error: %w", err)
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
	photo  Photo
	dryrun bool
	err    error
}

type cleanResultMsg struct {
	dir    string
	dryrun bool
	empty  bool
	err    error
}

func (r cleanResultMsg) Err() error { return r.err }

func (r cleanResultMsg) String() string {
	if r.err != nil {
		return fmt.Sprintf("%s %s: %v",
			errorStyle.Render("Failed to cleanup"),
			r.dir,
			r.err.Error())
	}
	if r.dryrun {
		return fmt.Sprintf("%s Would remove %s if empty",
			dryrunStyle.Render("(dryrun)"),
			r.dir,
		)
	}
	if r.empty {
		return fmt.Sprintf("Removed %s because empty", r.dir)
	} else {
		return fmt.Sprintf("Do not remove %s because NOT empty", r.dir)
	}
}

func (r analyzeResultMsg) Err() error { return r.err }

func (r renameResultMsg) Err() error { return r.err }

func (r renameResultMsg) String() string {
	if r.err != nil {
		return fmt.Sprintf("%s %s: %v",
			errorStyle.Render("Failed to rename"),
			r.photo.Name,
			r.err.Error())
	}
	if r.dryrun {
		return fmt.Sprintf("%s Would rename %s (%s)",
			dryrunStyle.Render("(dryrun)"),
			r.photo.Name,
			dryrunStyle.Render(r.photo.RenamedPath), // TODO: here
		)
	}
	return fmt.Sprintf("Renamed %s (%s)", r.photo.Name, r.photo.RenamedPath)
}

func (r analyzeResultMsg) String() string {
	if r.err != nil {
		return fmt.Sprintf("%s %s: %v",
			errorStyle.Render("Failed to get exif"),
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

type finishMsg struct{}

type model struct {
	progress       progress.Model
	spinner        spinner.Model
	quitting       bool
	results        []resultMsg
	errors         []resultMsg
	startTime      time.Time
	renamed, total int
	height         int
}

const maxHeight = 30

func newModel(total int) model {
	s := spinner.New()
	s.Style = spinnerStyle
	height := min(total, maxHeight)
	return model{
		height:    height,
		progress:  progress.New(progress.WithDefaultGradient()),
		spinner:   s,
		results:   make([]resultMsg, height),
		total:     total,
		startTime: time.Now(),
	}
}

func (m model) Init() tea.Cmd {
	return m.spinner.Tick
}

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
			if n := len(results); n >= m.height {
				m.results = append(results[n-m.height+1:], msg)
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
		s += "Renaming completed. " +
			durationStyle.Render("elapsed time: "+duration(time.Since(m.startTime)).String())
	} else {
		s += m.spinner.View() + " Processing photos..."
	}
	s += fmt.Sprintf(" (%d/%d)", m.renamed, m.total)

	s += "\n\n"

	for _, res := range m.results {
		switch res.(type) {
		case analyzeResultMsg, renameResultMsg, cleanResultMsg:
			s += res.String()
		default:
			s += dotStyle.Render(strings.Repeat(".", 30))
		}
		s += "\n"
	}

	s += "\n"

	showPb := m.total > 100 || time.Since(m.startTime).Seconds() > 3.0
	if percent := float64(m.renamed) / float64(m.total); percent < 1 && showPb {
		s += m.progress.ViewAs(percent)
		s += "\n"
	}

	return appStyle.Render(s)
}
