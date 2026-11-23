package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	// The test binary stands in for exiftool when the tests run it so
	if os.Getenv("NAMINATOR_FAKE_EXIFTOOL") == "1" {
		os.Exit(fakeExiftool())
	}
	os.Exit(m.Run())
}

// fakeExiftool speaks exiftool's -stay_open protocol, and answers by the
// name of each file:
//
//	noexif*   no shooting time
//	broken*   an error in the JSON, as for a file exiftool cannot parse
//	missing*  nothing on stdout and an error on stderr
//	quit*     exits without answering
//	numeric*  a model that is a number in the JSON
//	other     a shooting time and a model
func fakeExiftool() int {
	args := os.Args[1:]
	want := append([]string{"-stay_open", "True", "-@", "-", "-common_args", "-json", "-echo4", readyToken}, exiftoolTags...)
	if !slices.Equal(args, want) {
		fmt.Fprintf(os.Stderr, "unexpected args %q\n", args)
		return 2
	}
	in := bufio.NewScanner(os.Stdin)
	var cmd []string
	for in.Scan() {
		line := in.Text()
		if line != "-execute" {
			cmd = append(cmd, line)
			continue
		}
		if slices.Contains(cmd, "-stay_open") {
			return 0
		}
		if len(cmd) != 1 {
			fmt.Fprintf(os.Stderr, "unexpected command %q\n", cmd)
			return 2
		}
		path := cmd[0]
		cmd = nil
		name := filepath.Base(path)
		info := map[string]any{"SourceFile": path, "FileName": name}
		switch {
		case strings.HasPrefix(name, "noexif"):
		case strings.HasPrefix(name, "broken"):
			info["Error"] = "File format error"
		case strings.HasPrefix(name, "missing"):
			info = nil
			fmt.Fprintf(os.Stderr, "Error: File not found - %s\n", path)
		case strings.HasPrefix(name, "quit"):
			return 0
		case strings.HasPrefix(name, "numeric"):
			info["DateTimeOriginal"] = "2024:01:02 03:04:05"
			info["FileTypeExtension"] = "jpg"
			info["Model"] = json.Number("6")
		default:
			info["SubSecDateTimeOriginal"] = "2024:01:02 03:04:05.25+09:00"
			info["DateTimeOriginal"] = "2024:01:02 03:04:05"
			info["FileTypeExtension"] = "jpg"
			info["Model"] = "Camera  "
		}
		if info != nil {
			b, _ := json.Marshal([]any{info})
			fmt.Printf("%s\n", b)
		}
		fmt.Println(readyToken)
		fmt.Fprintln(os.Stderr, readyToken)
	}
	return 1
}

// startFakeExiftool starts the test binary as exiftool (see fakeExiftool).
func startFakeExiftool(t *testing.T) *exiftool {
	t.Helper()
	t.Setenv("NAMINATOR_FAKE_EXIFTOOL", "1")
	// Built with -race, the binary would wait a second before it exits
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	orig := exiftoolPath
	exiftoolPath = os.Args[0]
	t.Cleanup(func() { exiftoolPath = orig })
	e, err := startExiftool()
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func touch(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExiftoolExtract(t *testing.T) {
	e := startFakeExiftool(t)
	defer func() {
		if err := e.Close(); err != nil {
			t.Error(err)
		}
	}()
	dir := t.TempDir()

	// Several files one after another on one process
	for _, name := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		p := touch(t, dir, name)
		photo, err := e.Extract(p)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want := Photo{
			Name:      name,
			Path:      p,
			Dir:       dir,
			Extension: "jpg",
			Camera:    "Camera",
			CreatedAt: time.Date(2024, 1, 2, 3, 4, 5, 250_000_000, time.FixedZone("", 9*60*60)),
		}
		if photo.Name != want.Name || photo.Path != want.Path || photo.Dir != want.Dir ||
			photo.Extension != want.Extension || photo.Camera != want.Camera || !photo.CreatedAt.Equal(want.CreatedAt) {
			t.Errorf("Extract(%s) = %+v, want %+v", name, photo, want)
		}
	}

	photo, err := e.Extract(touch(t, dir, "numeric.jpg"))
	if err != nil || photo.Camera != "6" {
		t.Errorf("numeric model: got %q, %v", photo.Camera, err)
	}

	errs := []struct {
		name, want string
	}{
		{"noexif.jpg", "error on both SubSecDateTimeOriginal (key not found) and DateTimeOriginal: key not found"},
		{"broken.jpg", "exiftool: Error: File format error"},
		{"missing.jpg", "exiftool: Error: File not found - "},
	}
	for _, tt := range errs {
		p := touch(t, dir, tt.name)
		photo, err := e.Extract(p)
		if err == nil || !strings.HasPrefix(err.Error(), tt.want) {
			t.Errorf("Extract(%s) error = %v, want %q", tt.name, err, tt.want)
		}
		if photo.Name != tt.name {
			t.Errorf("Extract(%s) name = %q", tt.name, photo.Name)
		}
	}

	// The process still answers after the errors
	if _, err := e.Extract(touch(t, dir, "d.jpg")); err != nil {
		t.Errorf("after errors: %v", err)
	}
}

func TestExiftoolExtractNotAFile(t *testing.T) {
	e := startFakeExiftool(t)
	defer func() { _ = e.Close() }()
	dir := t.TempDir()
	if _, err := e.Extract(filepath.Join(dir, "none.jpg")); !os.IsNotExist(err) {
		t.Errorf("missing file: %v", err)
	}
	if _, err := e.Extract(dir); err == nil {
		t.Error("directory: no error")
	}
	if _, err := e.Extract(touch(t, dir, "a\nb.jpg")); err == nil {
		t.Error("line break: no error")
	}
}

func TestExiftoolExtractDashPath(t *testing.T) {
	e := startFakeExiftool(t)
	defer func() { _ = e.Close() }()
	t.Chdir(t.TempDir())
	touch(t, ".", "-a.jpg")
	photo, err := e.Extract("-a.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if photo.Name != "-a.jpg" {
		t.Errorf("name = %q", photo.Name)
	}
}

func TestExiftoolExited(t *testing.T) {
	e := startFakeExiftool(t)
	defer func() { _ = e.Close() }()
	dir := t.TempDir()
	if _, err := e.Extract(touch(t, dir, "quit.jpg")); err == nil {
		t.Fatal("no error when exiftool exits")
	}
	// Every later call fails at once rather than hanging
	_, err := e.Extract(touch(t, dir, "a.jpg"))
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Errorf("after exit: %v", err)
	}
}

// TestExiftoolReal runs the exiftool installed, if there is one.
func TestExiftoolReal(t *testing.T) {
	if _, err := exec.LookPath(exiftoolPath); err != nil {
		t.Skip("exiftool is not installed")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "photo.jpg")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(f, image.NewGray(image.Rect(0, 0, 8, 8)), nil); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(exiftoolPath, "-q", "-overwrite_original",
		"-DateTimeOriginal=2024:01:02 03:04:05", "-SubSecTimeOriginal=25",
		"-OffsetTimeOriginal=+09:00", "-Model=Camera", p).CombinedOutput()
	if err != nil {
		t.Fatalf("writing tags: %v: %s", err, out)
	}
	noexif := filepath.Join(dir, "noexif.jpg")
	if err := os.WriteFile(noexif, []byte("not a photo"), 0o644); err != nil {
		t.Fatal(err)
	}

	e, err := startExiftool()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := e.Close(); err != nil {
			t.Error(err)
		}
	}()
	photo, err := e.Extract(p)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2024, 1, 2, 3, 4, 5, 250_000_000, time.FixedZone("", 9*60*60))
	if photo.Name != "photo.jpg" || photo.Extension != "jpg" || photo.Camera != "Camera" || !photo.CreatedAt.Equal(want) {
		t.Errorf("Extract = %+v", photo)
	}
	if _, err := e.Extract(noexif); err == nil {
		t.Error("no error for a file without EXIF")
	}
	if _, err := e.Extract(p); err != nil {
		t.Errorf("after an error: %v", err)
	}
}
