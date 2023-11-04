package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/barasher/go-exiftool"
	"github.com/jessevdk/go-flags"
)

type Option struct {
	ParentDir string `short:"d" long:"parent-dir" description:"Target dir to move renamed photo" required:"false" default:"."`
}

type Photo struct {
	Name      string
	Path      string
	Extension string
	CreatedAt time.Time
}

func worker(inputs []string) <-chan Photo {
	output := make(chan Photo)
	var task sync.WaitGroup

	for _, input := range inputs {
		input := input
		task.Add(1)
		go func() {
			defer task.Done()
			p, err := exif(input)
			if err != nil {
				log.Println(err)
			}
			output <- p
		}()
	}

	go func() {
		task.Wait()
		close(output)
	}()

	return output
}

func main() {
	var opt Option
	args, err := flags.Parse(&opt)
	if err != nil {
		panic(err)
	}

	var photos []Photo
	ch := worker(args)
	for photo := range ch {
		fmt.Printf("Checking %s (time: %s)\n", photo.Name, photo.CreatedAt)
		photos = append(photos, photo)
	}

	// photos, err := exifBulk(args)
	// if err != nil {
	// 	panic(err)
	// }

	sort.Slice(photos, func(i, j int) bool {
		return photos[i].CreatedAt.Before(photos[j].CreatedAt)
	})
	for i, photo := range photos {
		newName := fmt.Sprintf("%s-%03d.%s",
			photo.CreatedAt.Format("2006-01-02"),
			i+1,
			photo.Extension,
		)
		moveTo := filepath.Join(opt.ParentDir, newName)
		fmt.Printf("Renaming %q to %q\n", photo.Path, moveTo)
		if err := os.Rename(photo.Path, moveTo); err != nil {
			log.Print(err)
			continue
		}
	}
}

// for i := range numbers {
// }

// func run(files []string) ([]Photo, error) {
// 	var mu = &sync.Mutex{}
// 	var photos []Photo
// 	eg := errgroup.Group{}
//
// 	for _, file := range files {
// 		file := file
// 		eg.Go(func() error {
// 			log.Printf("Checking %s", file)
// 			photo, err := exif(file)
// 			if err != nil {
// 				return err
// 			}
// 			mu.Lock()
// 			photos = append(photos, photo)
// 			mu.Unlock()
// 			return nil
// 		})
// 	}
// 	if err := eg.Wait(); err != nil {
// 		return photos, err
// 	}
// 	return photos, nil
// }

func exif(file string) (Photo, error) {
	et, err := exiftool.NewExiftool()
	if err != nil {
		return Photo{}, err
	}
	defer et.Close()

	fileInfos := et.ExtractMetadata(file)
	fileInfo := fileInfos[0]

	if fileInfo.Err != nil {
		return Photo{}, err
	}
	filename, err := fileInfo.GetString("FileName")
	if err != nil {
		return Photo{}, err
	}
	// Use it instead of DateTimeOriginal
	dateTime, err := fileInfo.GetString("SubSecDateTimeOriginal")
	if err != nil {
		return Photo{}, err
	}
	sourceFile, err := fileInfo.GetString("SourceFile")
	if err != nil {
		return Photo{}, err
	}
	ext, err := fileInfo.GetString("FileTypeExtension")
	if err != nil {
		return Photo{}, err
	}
	createdAt, err := time.Parse("2006:01:02 15:04:05.000-07:00", dateTime)
	if err != nil {
		return Photo{}, err
	}

	return Photo{
		Name:      filename,
		Path:      sourceFile,
		Extension: ext,
		CreatedAt: createdAt,
	}, nil
}

func exifBulk(files []string) ([]Photo, error) {
	et, err := exiftool.NewExiftool()
	if err != nil {
		return []Photo{}, err
	}
	defer et.Close()

	var photos []Photo

	fileInfos := et.ExtractMetadata(files...)
	for _, fileInfo := range fileInfos {
		if fileInfo.Err != nil {
			return []Photo{}, err
		}
		filename, err := fileInfo.GetString("FileName")
		if err != nil {
			return []Photo{}, err
		}
		// Use it instead of DateTimeOriginal
		dateTime, err := fileInfo.GetString("SubSecDateTimeOriginal")
		if err != nil {
			return []Photo{}, err
		}
		sourceFile, err := fileInfo.GetString("SourceFile")
		if err != nil {
			return []Photo{}, err
		}
		ext, err := fileInfo.GetString("FileTypeExtension")
		if err != nil {
			return []Photo{}, err
		}
		createdAt, err := time.Parse("2006:01:02 15:04:05.000-07:00", dateTime)
		if err != nil {
			return []Photo{}, err
		}

		photos = append(photos, Photo{
			Name:      filename,
			Path:      sourceFile,
			Extension: ext,
			CreatedAt: createdAt,
		})
	}

	return photos, nil
}
