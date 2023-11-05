package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/barasher/go-exiftool"
	"github.com/hashicorp/go-multierror"
	"github.com/jessevdk/go-flags"
	"github.com/k0kubun/go-ansi"
	"github.com/schollz/progressbar/v3"
	"golang.org/x/sync/errgroup"
)

type Option struct {
	ParentDir string `short:"d" long:"parent-dir" description:"Parent directory path to move renamed photos" required:"false" default:"."`
	Dryrun    bool   `short:"n" long:"dryrun" description:"Displays the operations that would be performed using the specified command without actually running them" required:"false"`
}

type Photo struct {
	Name      string
	Path      string
	Extension string
	CreatedAt time.Time
}

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] failed to rename files: %v\n", err)
		os.Exit(1)
	}
}

func runMain() error {
	ctx := context.Background()

	var opt Option
	args, err := flags.Parse(&opt)
	if err != nil {
		return err
	}

	var paths []string
	for _, arg := range args {
		// walkDir can traverse dirs or files
		files, err := walkDir(arg)
		if err != nil {
			return err
		}
		paths = append(paths, files...)
	}

	photos, err := getPhotos(ctx, paths)
	if err != nil {
		return err
	}

	sort.Slice(photos, func(i, j int) bool {
		return photos[i].CreatedAt.Before(photos[j].CreatedAt)
	})

	if err := os.MkdirAll(opt.ParentDir, 0755); err != nil {
		return err
	}

	var errs error
	for index, photo := range photos {
		newPath := filepath.Join(opt.ParentDir, fmt.Sprintf("%s-%03d.%s",
			photo.CreatedAt.Format("2006-01-02"),
			index+1,
			photo.Extension,
		))
		if opt.Dryrun {
			fmt.Printf("[INFO] (dryrun): Renaming %q to %q\n", photo.Path, newPath)
			continue
		}
		fmt.Printf("[INFO] Renaming %q to %q\n", photo.Path, newPath)
		if err := os.Rename(photo.Path, newPath); err != nil {
			// Use hashicorp/go-multierror instead of errors.Join (as of Go 1.20)
			// because this one is pretty good in output format.
			errs = multierror.Append(
				errs,
				fmt.Errorf("%s: failed to rename: %w", photo.Path, err))
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

func getPhotos(ctx context.Context, files []string) ([]Photo, error) {
	ch := make(chan Photo)
	eg, ctx := errgroup.WithContext(ctx)

	for _, file := range files {
		file := file
		eg.Go(func() error {
			photo, err := analyzeExifdata(file)
			if err != nil {
				return fmt.Errorf("%s: failed to get EXIF data: %w", file, err)
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
		bar.Add(1)
		photos = append(photos, photo)
	}

	// handle error in goroutines (secondary wait)
	return photos, eg.Wait()
}

func analyzeExifdata(file string) (Photo, error) {
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
		Extension: ext,
		CreatedAt: createdAt,
	}, nil
}
