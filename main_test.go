package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestDurationString(t *testing.T) {
	tests := []struct {
		name string
		d    duration
		want string
	}{
		{"zero", duration(0), "0.00s"},
		{"one second", duration(time.Second), "1.00s"},
		{"half second", duration(500 * time.Millisecond), "0.50s"},
		{"2.5 seconds", duration(2500 * time.Millisecond), "2.50s"},
		{"sub-millisecond", duration(100 * time.Microsecond), "0.00s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.d.String()
			if got != tt.want {
				t.Errorf("duration(%v).String() = %q, want %q", time.Duration(tt.d), got, tt.want)
			}
		})
	}
}

// --- Mocks ---

// mockFS records filesystem calls and can simulate errors.
type mockFS struct {
	renamed    map[string]string
	dirs       []string
	removed    []string
	renameErr  error
	mkdirErr   error
	removeErr  error
	statFunc   func(string) (os.FileInfo, error)
}

func newMockFS() *mockFS {
	return &mockFS{renamed: make(map[string]string)}
}

func (m *mockFS) Rename(oldpath, newpath string) error {
	if m.renameErr != nil {
		return m.renameErr
	}
	m.renamed[oldpath] = newpath
	return nil
}

func (m *mockFS) MkdirAll(path string, perm os.FileMode) error {
	if m.mkdirErr != nil {
		return m.mkdirErr
	}
	m.dirs = append(m.dirs, path)
	return nil
}

func (m *mockFS) RemoveAll(path string) error {
	if m.removeErr != nil {
		return m.removeErr
	}
	m.removed = append(m.removed, path)
	return nil
}

func (m *mockFS) Stat(name string) (os.FileInfo, error) {
	if m.statFunc != nil {
		return m.statFunc(name)
	}
	return os.Stat(name)
}

// mockExif returns predefined Photo data.
type mockExif struct {
	photos map[string]Photo
	err    error
}

func (m *mockExif) Extract(path string) (Photo, error) {
	if m.err != nil {
		return Photo{Name: filepath.Base(path), Path: path}, m.err
	}
	if p, ok := m.photos[path]; ok {
		return p, nil
	}
	return Photo{Name: filepath.Base(path), Path: path}, errors.New("no exif data")
}

// mockSender collects all sent messages for assertions.
type mockSender struct {
	mu   sync.Mutex
	msgs []tea.Msg
}

func (m *mockSender) Send(msg tea.Msg) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.msgs = append(m.msgs, msg)
}

func (m *mockSender) getMessages() []tea.Msg {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]tea.Msg, len(m.msgs))
	copy(cp, m.msgs)
	return cp
}

// --- buildNewPath tests (pure function, no mocks needed) ---

func TestBuildNewPath(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	createdAt := time.Date(2024, 3, 15, 14, 30, 45, 0, jst)

	photo := Photo{
		Name:      "test.jpg",
		Path:      "/photos/raw/test.jpg",
		Dir:       "/photos/raw",
		Extension: "jpg",
		CreatedAt: createdAt,
	}

	tests := []struct {
		name string
		opt  Option
		want string
	}{
		{
			name: "no grouping",
			opt:  Option{},
			want: "/photos/raw/2024-03-15_14-30-45.jpg",
		},
		{
			name: "group by date",
			opt:  Option{GroupByDate: true},
			want: "/photos/2024-03-15/2024-03-15_14-30-45.jpg",
		},
		{
			name: "group by ext",
			opt:  Option{GroupByExt: true},
			want: "/photos/jpg/2024-03-15_14-30-45.jpg",
		},
		{
			name: "group by date and ext (date first)",
			opt:  Option{GroupByDate: true, GroupByExt: true},
			want: "/photos/2024-03-15/jpg/2024-03-15_14-30-45.jpg",
		},
		{
			name: "group ext first",
			opt:  Option{GroupByDate: true, GroupByExt: true, GroupExtFirst: true},
			want: "/photos/jpg/2024-03-15/2024-03-15_14-30-45.jpg",
		},
		{
			name: "with dest dir",
			opt:  Option{DestDir: "/tmp/photos"},
			want: "/tmp/photos/2024-03-15_14-30-45.jpg",
		},
		{
			name: "dest dir with group by date",
			opt:  Option{DestDir: "/tmp/photos", GroupByDate: true},
			want: "/tmp/photos/2024-03-15/2024-03-15_14-30-45.jpg",
		},
		{
			name: "dest dir with group by date and ext",
			opt:  Option{DestDir: "/output", GroupByDate: true, GroupByExt: true},
			want: "/output/2024-03-15/jpg/2024-03-15_14-30-45.jpg",
		},
		{
			name: "dest dir with ext first",
			opt:  Option{DestDir: "/output", GroupByDate: true, GroupByExt: true, GroupExtFirst: true},
			want: "/output/jpg/2024-03-15/2024-03-15_14-30-45.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildNewPath(photo, tt.opt)
			if got != tt.want {
				t.Errorf("buildNewPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildNewPathDifferentExtensions(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)

	for _, ext := range []string{"jpg", "png", "heic", "arw", "cr2"} {
		t.Run(ext, func(t *testing.T) {
			photo := Photo{
				Dir:       "/photos/raw",
				Extension: ext,
				CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, jst),
			}
			got := buildNewPath(photo, Option{GroupByExt: true})
			want := "/photos/" + ext + "/2024-01-01_00-00-00." + ext
			if got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

// --- rename tests (with mockFS) ---

func TestRenameWithMockFS(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	createdAt := time.Date(2024, 3, 15, 14, 30, 45, 0, jst)

	photo := Photo{
		Name:      "test.jpg",
		Path:      "/photos/raw/test.jpg",
		Dir:       "/photos/raw",
		Extension: "jpg",
		CreatedAt: createdAt,
	}

	t.Run("normal rename calls MkdirAll then Rename", func(t *testing.T) {
		fs := newMockFS()
		cli := CLI{opt: Option{}, fs: fs, paths: newPathReserver()}
		got, dryrun, err := cli.rename(photo)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dryrun {
			t.Error("expected dryrun=false")
		}
		if got.RenamedPath != "/photos/raw/2024-03-15_14-30-45.jpg" {
			t.Errorf("RenamedPath = %q", got.RenamedPath)
		}
		if fs.renamed[photo.Path] != got.RenamedPath {
			t.Error("fs.Rename was not called correctly")
		}
		if len(fs.dirs) != 1 {
			t.Errorf("MkdirAll called %d times, want 1", len(fs.dirs))
		}
	})

	t.Run("dryrun skips all fs operations", func(t *testing.T) {
		fs := newMockFS()
		cli := CLI{opt: Option{Dryrun: true}, fs: fs, paths: newPathReserver()}
		got, dryrun, err := cli.rename(photo)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !dryrun {
			t.Error("expected dryrun=true")
		}
		// Path should still be computed
		if got.RenamedPath == "" {
			t.Error("RenamedPath should be set even in dryrun")
		}
		if len(fs.renamed) != 0 {
			t.Error("fs.Rename should not be called in dryrun")
		}
		if len(fs.dirs) != 0 {
			t.Error("fs.MkdirAll should not be called in dryrun")
		}
	})

	t.Run("rename error propagates", func(t *testing.T) {
		fs := newMockFS()
		fs.renameErr = errors.New("permission denied")
		cli := CLI{opt: Option{}, fs: fs, paths: newPathReserver()}
		_, _, err := cli.rename(photo)
		if err == nil || err.Error() != "permission denied" {
			t.Errorf("expected 'permission denied', got %v", err)
		}
	})

	t.Run("MkdirAll error propagates without renaming", func(t *testing.T) {
		fs := newMockFS()
		fs.mkdirErr = errors.New("read-only file system")
		cli := CLI{opt: Option{}, fs: fs, paths: newPathReserver()}
		_, _, err := cli.rename(photo)
		if err == nil || err.Error() != "read-only file system" {
			t.Errorf("expected 'read-only file system', got %v", err)
		}
		if len(fs.renamed) != 0 {
			t.Error("fs.Rename should not be called when MkdirAll fails")
		}
	})

	t.Run("MkdirAll creates parent of RenamedPath", func(t *testing.T) {
		fs := newMockFS()
		cli := CLI{opt: Option{GroupByDate: true}, fs: fs, paths: newPathReserver()}
		got, _, _ := cli.rename(photo)
		expectedDir := filepath.Dir(got.RenamedPath)
		if len(fs.dirs) != 1 || fs.dirs[0] != expectedDir {
			t.Errorf("MkdirAll dir = %v, want %q", fs.dirs, expectedDir)
		}
	})
}

func TestRenameCollision(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	createdAt := time.Date(2024, 3, 15, 14, 30, 45, 0, jst)
	newPhoto := func(name string) Photo {
		return Photo{
			Name:      name,
			Path:      "/photos/raw/" + name,
			Dir:       "/photos/raw",
			Extension: "jpg",
			CreatedAt: createdAt,
		}
	}

	for _, dryrun := range []bool{false, true} {
		t.Run(map[bool]string{false: "rename", true: "dryrun"}[dryrun], func(t *testing.T) {
			cli := CLI{opt: Option{Dryrun: dryrun}, fs: newMockFS(), paths: newPathReserver()}
			var got []string
			for _, name := range []string{"a.jpg", "b.jpg", "c.jpg"} {
				photo, _, err := cli.rename(newPhoto(name))
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				got = append(got, photo.RenamedPath)
			}
			want := []string{
				"/photos/raw/2024-03-15_14-30-45.jpg",
				"/photos/raw/2024-03-15_14-30-45_1.jpg",
				"/photos/raw/2024-03-15_14-30-45_2.jpg",
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("RenamedPath[%d] = %q, want %q", i, got[i], want[i])
				}
			}
		})
	}
}

func TestPathReserver(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, nil, 0644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	want := filepath.Join(dir, "2024-03-15_14-30-45.jpg")

	t.Run("skips a path taken by another file", func(t *testing.T) {
		write("2024-03-15_14-30-45.jpg")
		src := write("other.jpg")
		got := newPathReserver().reserve(osFS{}, src, want)
		if exp := filepath.Join(dir, "2024-03-15_14-30-45_1.jpg"); got != exp {
			t.Errorf("reserve() = %q, want %q", got, exp)
		}
	})

	t.Run("keeps the path the source already has", func(t *testing.T) {
		src := write("2024-03-15_14-30-45.jpg")
		got := newPathReserver().reserve(osFS{}, src, want)
		if got != want {
			t.Errorf("reserve() = %q, want %q", got, want)
		}
	})
}

func TestParseExifTime(t *testing.T) {
	tests := []struct {
		value string
		// want is the clock time the name is made of, and offset the UTC
		// offset in seconds the result should keep
		want   string
		offset int
	}{
		{"2024:01:02 15:04:05", "2024-01-02 15:04:05", 0},
		{"2024:01:02 15:04:05+09:00", "2024-01-02 15:04:05", 9 * 60 * 60},
		{"2024:01:02 15:04:05.123+09:00", "2024-01-02 15:04:05.123", 9 * 60 * 60},
		{"2024:01:02 15:04:05.12+09:00", "2024-01-02 15:04:05.12", 9 * 60 * 60},
		{"2024:01:02 15:04:05.1234-05:00", "2024-01-02 15:04:05.1234", -5 * 60 * 60},
		{"2024:01:02 15:04:05.5Z", "2024-01-02 15:04:05.5", 0},
		{"2024:01:02 15:04:05.123", "2024-01-02 15:04:05.123", 0},
		// Taken in Paris: kept as the local time there, not converted
		{"2024:07:14 23:30:00+02:00", "2024-07-14 23:30:00", 2 * 60 * 60},
		// A clock time skipped by daylight saving time in many zones
		{"2024:03:10 02:30:00", "2024-03-10 02:30:00", 0},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, err := parseExifTime(tt.value)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s := got.Format("2006-01-02 15:04:05.999999999"); s != tt.want {
				t.Errorf("parseExifTime(%q) = %q, want %q", tt.value, s, tt.want)
			}
			if _, offset := got.Zone(); offset != tt.offset {
				t.Errorf("parseExifTime(%q) offset = %d, want %d", tt.value, offset, tt.offset)
			}
		})
	}

	if _, err := parseExifTime("not a time"); err == nil {
		t.Error("expected an error for an invalid value")
	}
}

// countingExif wraps mockExif and records how many files it extracted.
type countingExif struct {
	mockExif
	mu    sync.Mutex
	count int
}

func (c *countingExif) Extract(path string) (Photo, error) {
	c.mu.Lock()
	c.count++
	c.mu.Unlock()
	return c.mockExif.Extract(path)
}

func TestProcessAll(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	photos := map[string]Photo{}
	var images []string
	for i := range 20 {
		path := filepath.Join("/photos/raw", fmt.Sprintf("%02d.jpg", i))
		images = append(images, path)
		photos[path] = Photo{
			Name:      filepath.Base(path),
			Path:      path,
			Dir:       "/photos/raw",
			Extension: "jpg",
			CreatedAt: time.Date(2024, 3, 15, 14, 30, i, 0, jst),
		}
	}
	// A file without EXIF data
	images = append(images, "/photos/raw/noexif.jpg")

	exifs := []*countingExif{
		{mockExif: mockExif{photos: photos}},
		{mockExif: mockExif{photos: photos}},
		{mockExif: mockExif{photos: photos}},
	}
	sender := &mockSender{}
	cli := CLI{
		opt:    Option{Dryrun: true},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		sender: sender,
		images: images,
		fs:     newMockFS(),
		paths:  newPathReserver(),
	}
	for _, e := range exifs {
		cli.exifs = append(cli.exifs, e)
	}
	cli.processAll()

	total := 0
	for _, e := range exifs {
		total += e.count
	}
	if total != len(images) {
		t.Errorf("extracted %d files, want %d", total, len(images))
	}

	var exifOK, exifFailed, renamed int
	for _, msg := range sender.getMessages() {
		switch m := msg.(type) {
		case exifResultMsg:
			if m.err != nil {
				exifFailed++
			} else {
				exifOK++
			}
		case renameResultMsg:
			renamed++
		}
	}
	if exifOK != 20 || exifFailed != 1 || renamed != 20 {
		t.Errorf("exif ok=%d failed=%d renamed=%d, want 20, 1, 20", exifOK, exifFailed, renamed)
	}
}

// --- clean tests (with mockFS + mockSender) ---

func TestClean(t *testing.T) {
	t.Run("does nothing when Clean=false", func(t *testing.T) {
		sender := &mockSender{}
		cli := CLI{opt: Option{Clean: false}, sender: sender, fs: newMockFS()}
		cli.clean([]string{"/some/dir"})

		if len(sender.getMessages()) != 0 {
			t.Error("should not send any messages when Clean=false")
		}
	})

	t.Run("skips non-existent paths silently", func(t *testing.T) {
		fs := newMockFS()
		fs.statFunc = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		sender := &mockSender{}
		cli := CLI{opt: Option{Clean: true}, sender: sender, fs: fs}
		cli.clean([]string{"/nonexistent"})

		if len(sender.getMessages()) != 0 {
			t.Error("should silently skip non-existent paths")
		}
	})

	t.Run("skips non-directory paths", func(t *testing.T) {
		// Create a real temp file to get valid FileInfo for a file (not dir)
		tmp := t.TempDir()
		file := filepath.Join(tmp, "file.txt")
		if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}

		sender := &mockSender{}
		cli := CLI{opt: Option{Clean: true}, sender: sender, fs: osFS{}}
		cli.clean([]string{file})

		if len(sender.getMessages()) != 0 {
			t.Error("should skip files (non-directories)")
		}
	})

	t.Run("removes empty directory", func(t *testing.T) {
		dir := t.TempDir()
		emptyDir := filepath.Join(dir, "empty")
		if err := os.MkdirAll(emptyDir, 0755); err != nil {
			t.Fatal(err)
		}

		fs := newMockFS()
		// Use real Stat so emptyDirs works
		fs.statFunc = func(name string) (os.FileInfo, error) { return os.Stat(name) }
		sender := &mockSender{}
		cli := CLI{opt: Option{Clean: true}, sender: sender, fs: fs}
		cli.clean([]string{emptyDir})

		msgs := sender.getMessages()
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message, got %d", len(msgs))
		}
		msg, ok := msgs[0].(cleanResultMsg)
		if !ok {
			t.Fatalf("expected cleanResultMsg, got %T", msgs[0])
		}
		if !msg.empty {
			t.Error("expected empty=true")
		}
		if msg.err != nil {
			t.Errorf("unexpected error: %v", msg.err)
		}
		if len(fs.removed) != 1 || fs.removed[0] != emptyDir {
			t.Errorf("RemoveAll called with %v, want [%s]", fs.removed, emptyDir)
		}
	})

	t.Run("does not remove non-empty directory", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}

		fs := newMockFS()
		fs.statFunc = func(name string) (os.FileInfo, error) { return os.Stat(name) }
		sender := &mockSender{}
		cli := CLI{opt: Option{Clean: true}, sender: sender, fs: fs}
		cli.clean([]string{dir})

		msgs := sender.getMessages()
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message, got %d", len(msgs))
		}
		msg := msgs[0].(cleanResultMsg)
		if msg.empty {
			t.Error("expected empty=false")
		}
		if len(fs.removed) != 0 {
			t.Error("RemoveAll should not be called for non-empty dir")
		}
	})

	t.Run("dryrun does not actually remove", func(t *testing.T) {
		dir := t.TempDir()
		emptyDir := filepath.Join(dir, "empty")
		if err := os.MkdirAll(emptyDir, 0755); err != nil {
			t.Fatal(err)
		}

		fs := newMockFS()
		fs.statFunc = func(name string) (os.FileInfo, error) { return os.Stat(name) }
		sender := &mockSender{}
		cli := CLI{opt: Option{Clean: true, Dryrun: true}, sender: sender, fs: fs}
		cli.clean([]string{emptyDir})

		msgs := sender.getMessages()
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message, got %d", len(msgs))
		}
		msg := msgs[0].(cleanResultMsg)
		if !msg.dryrun {
			t.Error("expected dryrun=true in message")
		}
		if len(fs.removed) != 0 {
			t.Error("RemoveAll should not be called in dryrun")
		}
	})

	t.Run("RemoveAll error is sent as message", func(t *testing.T) {
		dir := t.TempDir()
		emptyDir := filepath.Join(dir, "empty")
		if err := os.MkdirAll(emptyDir, 0755); err != nil {
			t.Fatal(err)
		}

		fs := newMockFS()
		fs.statFunc = func(name string) (os.FileInfo, error) { return os.Stat(name) }
		fs.removeErr = errors.New("rm failed")
		sender := &mockSender{}
		cli := CLI{opt: Option{Clean: true}, sender: sender, fs: fs}
		cli.clean([]string{emptyDir})

		msgs := sender.getMessages()
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message, got %d", len(msgs))
		}
		msg := msgs[0].(cleanResultMsg)
		if msg.err == nil || msg.err.Error() != "rm failed" {
			t.Errorf("expected 'rm failed' error, got %v", msg.err)
		}
	})

	t.Run("processes multiple paths", func(t *testing.T) {
		dir := t.TempDir()
		empty1 := filepath.Join(dir, "a")
		empty2 := filepath.Join(dir, "b")
		if err := os.MkdirAll(empty1, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(empty2, 0755); err != nil {
			t.Fatal(err)
		}

		fs := newMockFS()
		fs.statFunc = func(name string) (os.FileInfo, error) { return os.Stat(name) }
		sender := &mockSender{}
		cli := CLI{opt: Option{Clean: true}, sender: sender, fs: fs}
		cli.clean([]string{empty1, empty2})

		msgs := sender.getMessages()
		if len(msgs) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(msgs))
		}
		if len(fs.removed) != 2 {
			t.Errorf("RemoveAll called %d times, want 2", len(fs.removed))
		}
	})

	t.Run("removes empty subdirectories and junk files for real", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "DCIM")
		makeTree(t, root, "100MSDCF/.DS_Store", "2024-03-15/a.jpg")

		sender := &mockSender{}
		cli := CLI{opt: Option{Clean: true}, sender: sender, fs: osFS{}}
		cli.clean([]string{root})

		var got []string
		for _, msg := range sender.getMessages() {
			m := msg.(cleanResultMsg)
			if m.err != nil {
				t.Errorf("unexpected error for %s: %v", m.dir, m.err)
			}
			got = append(got, fmt.Sprintf("%s:%v", m.dir, m.empty))
		}
		if want := "DCIM/100MSDCF:true,DCIM:false"; strings.Join(got, ",") != want {
			t.Errorf("messages = %v, want %s", got, want)
		}
		if _, err := os.Stat(filepath.Join(root, "100MSDCF")); !os.IsNotExist(err) {
			t.Error("DCIM/100MSDCF should be removed")
		}
		if _, err := os.Stat(filepath.Join(root, "2024-03-15", "a.jpg")); err != nil {
			t.Errorf("DCIM/2024-03-15/a.jpg should be kept: %v", err)
		}
	})

	t.Run("removes a directory with only junk files left", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "card")
		makeTree(t, root, ".DS_Store")

		sender := &mockSender{}
		cli := CLI{opt: Option{Clean: true}, sender: sender, fs: osFS{}}
		cli.clean([]string{root})

		msgs := sender.getMessages()
		if len(msgs) != 1 || !msgs[0].(cleanResultMsg).empty {
			t.Fatalf("expected one removed message, got %v", msgs)
		}
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Error("card should be removed")
		}
	})

	t.Run("dryrun predicts with the photos it would move", func(t *testing.T) {
		parent := t.TempDir()
		root := filepath.Join(parent, "DCIM")
		makeTree(t, root, "100MSDCF/a.jpg", "100MSDCF/.DS_Store")
		createdAt := time.Date(2024, 3, 15, 14, 30, 45, 0, time.UTC)

		sender := &mockSender{}
		cli := CLI{
			opt:    Option{Clean: true, Dryrun: true, GroupByDate: true},
			sender: sender,
			fs:     osFS{},
			paths:  newPathReserver(),
			moves:  newPendingMoves(),
		}
		if _, _, err := cli.rename(Photo{
			Name:      "a.jpg",
			Path:      filepath.Join(root, "100MSDCF", "a.jpg"),
			Dir:       filepath.Join(root, "100MSDCF"),
			Extension: "jpg",
			CreatedAt: createdAt,
		}); err != nil {
			t.Fatal(err)
		}
		cli.clean([]string{root})

		var got []string
		for _, msg := range sender.getMessages() {
			m := msg.(cleanResultMsg)
			got = append(got, fmt.Sprintf("%s:%v:%v", m.dir, m.dryrun, m.empty))
		}
		if want := "DCIM/100MSDCF:true:true,DCIM:true:false"; strings.Join(got, ",") != want {
			t.Errorf("messages = %v, want %s", got, want)
		}
		if _, err := os.Stat(filepath.Join(root, "100MSDCF", "a.jpg")); err != nil {
			t.Errorf("dryrun should not touch files: %v", err)
		}
	})
}

// --- Filesystem utility tests ---

// makeTree creates files (and their directories) under root, and directories
// for paths ending with "/".
func makeTree(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		path := filepath.Join(root, p)
		if strings.HasSuffix(p, "/") {
			if err := os.MkdirAll(path, 0755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEmptyDirs(t *testing.T) {
	tests := []struct {
		name      string
		tree      []string
		wantEmpty bool
		wantDirs  []string
	}{
		{"empty", nil, true, []string{"."}},
		{"only junk files", []string{".DS_Store", "Thumbs.db", "desktop.ini"}, true, []string{"."}},
		{"a file", []string{"a.jpg"}, false, nil},
		{"nested empty directories", []string{"a/b/", "a/.DS_Store", "c/"}, true, []string{"."}},
		{
			"empty directories next to a file",
			[]string{"100MSDCF/.DS_Store", "2024-03-15/a.jpg", "2024-03-15/old/", "misc/x/", "misc/y/"},
			false,
			[]string{"100MSDCF", "2024-03-15/old", "misc"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			makeTree(t, root, tt.tree...)
			empty, dirs, err := emptyDirs(root, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if empty != tt.wantEmpty {
				t.Errorf("empty = %v, want %v", empty, tt.wantEmpty)
			}
			var got []string
			for _, dir := range dirs {
				rel, _ := filepath.Rel(root, dir)
				got = append(got, rel)
			}
			if strings.Join(got, ",") != strings.Join(tt.wantDirs, ",") {
				t.Errorf("dirs = %v, want %v", got, tt.wantDirs)
			}
		})
	}

	t.Run("predicts with pending moves", func(t *testing.T) {
		tests := []struct {
			name      string
			moves     [][2]string // src, dst relative to the parent of root
			wantEmpty bool
			wantDirs  []string
		}{
			{"nothing moved", nil, false, nil},
			{
				"all moved out",
				[][2]string{{"DCIM/100MSDCF/a.jpg", "2024-03-15/a.jpg"}, {"DCIM/100MSDCF/b.jpg", "2024-03-15/b.jpg"}},
				true, []string{"."},
			},
			{
				"moved into a new directory under root",
				[][2]string{{"DCIM/100MSDCF/a.jpg", "DCIM/2024-03-15/a.jpg"}, {"DCIM/100MSDCF/b.jpg", "DCIM/2024-03-15/b.jpg"}},
				false, []string{"100MSDCF"},
			},
			{
				"renamed in place",
				[][2]string{{"DCIM/100MSDCF/a.jpg", "DCIM/100MSDCF/x.jpg"}, {"DCIM/100MSDCF/b.jpg", "DCIM/100MSDCF/y.jpg"}},
				false, nil,
			},
			{
				"one file left",
				[][2]string{{"DCIM/100MSDCF/a.jpg", "2024-03-15/a.jpg"}},
				false, nil,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				parent := t.TempDir()
				root := filepath.Join(parent, "DCIM")
				makeTree(t, root, "100MSDCF/a.jpg", "100MSDCF/b.jpg", "100MSDCF/.DS_Store")
				moves := newPendingMoves()
				for _, m := range tt.moves {
					moves.add(filepath.Join(parent, m[0]), filepath.Join(parent, m[1]))
				}
				empty, dirs, err := emptyDirs(root, moves)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if empty != tt.wantEmpty {
					t.Errorf("empty = %v, want %v", empty, tt.wantEmpty)
				}
				var got []string
				for _, dir := range dirs {
					rel, _ := filepath.Rel(root, dir)
					got = append(got, rel)
				}
				if strings.Join(got, ",") != strings.Join(tt.wantDirs, ",") {
					t.Errorf("dirs = %v, want %v", got, tt.wantDirs)
				}
			})
		}
	})

	t.Run("non-existent dir", func(t *testing.T) {
		if _, _, err := emptyDirs("/nonexistent/path", nil); err == nil {
			t.Error("expected error for non-existent dir")
		}
	})
}

func TestWalkDir(t *testing.T) {
	dir := t.TempDir()

	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.txt", "b.jpg"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(subDir, "c.png"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	files, err := walkDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Errorf("walkDir() returned %d files, want 3: %v", len(files), files)
	}

	for _, f := range files {
		fi, _ := os.Stat(f)
		if fi.IsDir() {
			t.Errorf("walkDir() should not include directories, got %s", f)
		}
	}
}

func TestWalkDirEmpty(t *testing.T) {
	dir := t.TempDir()
	files, err := walkDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestWalkDirNonExistent(t *testing.T) {
	_, err := walkDir("/nonexistent/path")
	if err == nil {
		t.Error("expected error for non-existent path")
	}
}
