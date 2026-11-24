package photo

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/barasher/go-exiftool"
)

// Extractor extracts EXIF metadata from a file.
type Extractor interface {
	Extract(path string) (Photo, error)
}

// Exiftool is the real Extractor using go-exiftool. It keeps
// one exiftool process running (stay_open) and reuses it for every file.
type Exiftool struct {
	et *exiftool.Exiftool
}

func NewExiftool() (Exiftool, error) {
	et, err := exiftool.NewExiftool()
	if err != nil {
		return Exiftool{}, fmt.Errorf("failed to run exiftool: %w", err)
	}
	return Exiftool{et: et}, nil
}

func (e Exiftool) Extract(path string) (Photo, error) {
	return getExifdata(e.et, path)
}

func (e Exiftool) Close() error {
	return e.et.Close()
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
	return photoOf(photo, fileInfos[0])
}

// photoOf fills photo from what exiftool read of it.
func photoOf(photo Photo, fileInfo exiftool.FileMetadata) (Photo, error) {
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
		switch {
		case errors.Is(err, exiftool.ErrKeyNotFound) && errors.Is(fallbackErr, exiftool.ErrKeyNotFound):
			// a screenshot, or a photo whose EXIF lost its date
			return photo, ErrNoShootingTime
		case fallbackErr != nil:
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
		Dir:       filepath.Dir(photo.Path),
		Extension: ext,
		Camera:    strings.TrimSpace(camera),
		CreatedAt: createdAt,
	}, nil
}

// ErrNoShootingTime is the error of a photo whose EXIF has no shooting time.
var ErrNoShootingTime = errors.New("no shooting time in its EXIF; skipped")

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
