package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/barasher/go-exiftool"
	"github.com/hashicorp/go-multierror"
	"github.com/jessevdk/go-flags"
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
			fmt.Printf("[INFO] Checking %s (time: %s)\n", photo.Name, photo.CreatedAt)
			select {
			case ch <- photo:
			case <-ctx.Done():
				return ctx.Err()
			}
			return nil
		})
	}

	// https://devlights.hatenablog.com/entry/2020/03/10/112904
	go func() {
		_ = eg.Wait()
		close(ch)
	}()

	var photos []Photo
	for photo := range ch {
		photos = append(photos, photo)
	}

	// secondaly wait
	return photos, eg.Wait()
}

func runMain() error {
	ctx := context.Background()

	var opt Option
	args, err := flags.Parse(&opt)
	if err != nil {
		return err
	}

	photos, err := getPhotos(ctx, args)
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
			errs = multierror.Append(
				errs,
				fmt.Errorf("%s: failed to rename: %w", photo.Path, err))
		}
	}

	err, ok := errs.(*multierror.Error)
	if !ok {
		return errors.New("multierror assertion error")
	}

	return err
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
