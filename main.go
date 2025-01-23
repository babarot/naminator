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
	"github.com/gabriel-vasile/mimetype"
	"github.com/hashicorp/go-multierror"
	"github.com/jessevdk/go-flags"
	"github.com/k0kubun/go-ansi"
	"github.com/schollz/progressbar/v3"
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

	var allPhotos []Photo
	for _, arg := range args {
		photos, err := getPhotos(ctx, arg)
		if err != nil {
			return err
		}
		allPhotos = append(allPhotos, photos...)
	}

	sort.Slice(allPhotos, func(i, j int) bool {
		return allPhotos[i].CreatedAt.Before(allPhotos[j].CreatedAt)
	})

	var errs error
	var newPath string
	for index, photo := range allPhotos {
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
			fmt.Printf("[INFO] (dryrun): Renaming %q to %q\n", photo.Path, newPath)
			continue
		}
		fmt.Printf("[INFO] Renaming %q to %q\n", photo.Path, newPath)
		_ = os.MkdirAll(dest, 0755)
		if err := os.Rename(photo.Path, newPath); err != nil {
			// Use hashicorp/go-multierror instead of errors.Join (as of Go 1.20)
			// because this one is pretty good in output format.
			errs = multierror.Append(
				errs,
				fmt.Errorf("%s: failed to rename: %w", photo.Path, err))
		}
	}

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

func getPhotos(ctx context.Context, dir string) ([]Photo, error) {
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
			photo, err := analyzeExifdata(dir, file)
			if err != nil {
				log.Print(fmt.Errorf("%s: failed to get EXIF data: %w", file, err))
				return nil
			}
			select {
			case ch <- photo:
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

	bar := progressbar.NewOptions(len(files),
		progressbar.OptionSetWriter(ansi.NewAnsiStdout()),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionSetWidth(20),
		progressbar.OptionSetDescription("[INFO] Checking exif on photos..."),
		progressbar.OptionOnCompletion(func() {
			fmt.Fprint(os.Stdout, "\n")
		}),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}))

	var photos []Photo
	for photo := range ch {
		_ = bar.Add(1)
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
