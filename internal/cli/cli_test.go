package cli

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/babarot/naminator/internal/photo"
	"github.com/babarot/naminator/internal/rename"
)

// --- Mocks ---

// mockFS records filesystem calls and can simulate errors.
type mockFS struct {
	renamed   map[string]string
	dirs      []string
	removed   []string
	renameErr error
	mkdirErr  error
	removeErr error
	statFunc  func(string) (os.FileInfo, error)
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

func (m *mockFS) ReadDir(name string) ([]os.DirEntry, error) {
	return os.ReadDir(name)
}

func (m *mockFS) Stat(name string) (os.FileInfo, error) {
	if m.statFunc != nil {
		return m.statFunc(name)
	}
	return os.Stat(name)
}

// mockExif returns predefined Photo data.
type mockExif struct {
	photos map[string]photo.Photo
	err    error
}

func (m *mockExif) Extract(path string) (photo.Photo, error) {
	if m.err != nil {
		return photo.Photo{Name: filepath.Base(path), Path: path}, m.err
	}
	if p, ok := m.photos[path]; ok {
		return p, nil
	}
	return photo.Photo{Name: filepath.Base(path), Path: path}, errors.New("no exif data")
}

// fakeTracker records what the work tells, as the events below.
type fakeTracker struct {
	mu     sync.Mutex
	events []any
}

type (
	readEvent struct {
		image string
		photo photo.Photo
		err   error
	}
	renamedEvent struct {
		photo  photo.Photo
		dryrun bool
		err    error
	}
	warnedEvent  struct{ name, text string }
	cleanedEvent struct {
		dir           string
		dryrun, empty bool
		err           error
	}
)

func (t *fakeTracker) add(e any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, e)
}

func (t *fakeTracker) getEvents() []any {
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Clone(t.events)
}

func (t *fakeTracker) ReadStart(string) {}

func (t *fakeTracker) ReadDone(image string, p photo.Photo, _ time.Duration, err error) {
	t.add(readEvent{image, p, err})
}
func (t *fakeTracker) RenameStart([]photo.Photo, string, bool, bool) {}
func (t *fakeTracker) Renamed(p photo.Photo, dry bool, err error) {
	t.add(renamedEvent{p, dry, err})
}
func (t *fakeTracker) Warn(name, text string) { t.add(warnedEvent{name, text}) }
func (t *fakeTracker) CleanStart(bool)        {}
func (t *fakeTracker) Cleaned(dir string, dry, empty bool, err error) {
	t.add(cleanedEvent{dir, dry, empty, err})
}
func (t *fakeTracker) Finish() {}

// --- buildNewPath tests (pure function, no mocks needed) ---

func TestBuildNewPath(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	createdAt := time.Date(2024, 3, 15, 14, 30, 45, 0, jst)

	photo := photo.Photo{
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
			photo := photo.Photo{
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

	photo := photo.Photo{
		Name:      "test.jpg",
		Path:      "/photos/raw/test.jpg",
		Dir:       "/photos/raw",
		Extension: "jpg",
		CreatedAt: createdAt,
	}

	t.Run("normal rename calls MkdirAll then Rename", func(t *testing.T) {
		fs := newMockFS()
		cli := CLI{opt: Option{}, fs: fs, paths: rename.NewReserver()}
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
		cli := CLI{opt: Option{Dryrun: true}, fs: fs, paths: rename.NewReserver()}
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
		cli := CLI{opt: Option{}, fs: fs, paths: rename.NewReserver()}
		_, _, err := cli.rename(photo)
		if err == nil || err.Error() != "permission denied" {
			t.Errorf("expected 'permission denied', got %v", err)
		}
	})

	t.Run("MkdirAll error propagates without renaming", func(t *testing.T) {
		fs := newMockFS()
		fs.mkdirErr = errors.New("read-only file system")
		cli := CLI{opt: Option{}, fs: fs, paths: rename.NewReserver()}
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
		cli := CLI{opt: Option{GroupByDate: true}, fs: fs, paths: rename.NewReserver()}
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
	newPhoto := func(name string) photo.Photo {
		return photo.Photo{
			Name:      name,
			Path:      "/photos/raw/" + name,
			Dir:       "/photos/raw",
			Extension: "jpg",
			CreatedAt: createdAt,
		}
	}

	for _, dryrun := range []bool{false, true} {
		t.Run(map[bool]string{false: "rename", true: "dryrun"}[dryrun], func(t *testing.T) {
			cli := CLI{opt: Option{Dryrun: dryrun}, fs: newMockFS(), paths: rename.NewReserver()}
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

// countingExif wraps mockExif and records how many files it extracted.
type countingExif struct {
	mockExif
	mu    sync.Mutex
	count int
}

func (c *countingExif) Extract(path string) (photo.Photo, error) {
	c.mu.Lock()
	c.count++
	c.mu.Unlock()
	return c.mockExif.Extract(path)
}

func TestProcessAll(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	photos := map[string]photo.Photo{}
	var images []string
	for i := range 20 {
		path := filepath.Join("/photos/raw", fmt.Sprintf("%02d.jpg", i))
		images = append(images, path)
		photos[path] = photo.Photo{
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
	sender := &fakeTracker{}
	cli := CLI{
		opt:     Option{Dryrun: true},
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		tracker: sender,
		images:  images,
		fs:      newMockFS(),
		paths:   rename.NewReserver(),
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
	for _, msg := range sender.getEvents() {
		switch m := msg.(type) {
		case readEvent:
			if m.err != nil {
				exifFailed++
			} else {
				exifOK++
			}
		case renamedEvent:
			renamed++
		}
	}
	if exifOK != 20 || exifFailed != 1 || renamed != 20 {
		t.Errorf("exif ok=%d failed=%d renamed=%d, want 20, 1, 20", exifOK, exifFailed, renamed)
	}
}

func TestProcessAllNamesBurstInOrder(t *testing.T) {
	second := time.Date(2025, 1, 29, 21, 7, 43, 0, time.UTC)
	tests := []struct {
		name string
		// shots maps an original file number to its sub-second time
		shots map[string]time.Duration
		// want is the shot numbers in the order they should be numbered
		want []string
	}{
		{"same second without sub-seconds", map[string]time.Duration{"569": 0, "570": 0, "571": 0}, []string{"569", "570", "571"}},
		{"sub-seconds win over file names", map[string]time.Duration{"569": 800 * time.Millisecond, "570": 100 * time.Millisecond, "571": 500 * time.Millisecond}, []string{"570", "571", "569"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			photos := map[string]photo.Photo{}
			var images []string
			for num, subsec := range tt.shots {
				for _, ext := range []string{"arw", "heif"} {
					name := "A7C00" + num + "." + strings.ToUpper(ext)
					path := filepath.Join("/DCIM/100MSDCF", name)
					images = append(images, path)
					photos[path] = photo.Photo{
						Name:      name,
						Path:      path,
						Dir:       "/DCIM/100MSDCF",
						Extension: ext,
						CreatedAt: second.Add(subsec),
					}
				}
			}

			// Workers finish in a different order each time, so repeat
			for range 20 {
				sender := &fakeTracker{}
				cli := CLI{
					opt:     Option{Dryrun: true, GroupByExt: true},
					logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
					tracker: sender,
					images:  images,
					fs:      newMockFS(),
					paths:   rename.NewReserver(),
				}
				for range 4 {
					cli.exifs = append(cli.exifs, &mockExif{photos: photos})
				}
				cli.processAll()

				got := map[string]string{}
				for _, msg := range sender.getEvents() {
					if m, ok := msg.(renamedEvent); ok {
						got[m.photo.RenamedPath] = m.photo.Name
					}
				}
				for i, num := range tt.want {
					suffix := ""
					if i > 0 {
						suffix = fmt.Sprintf("_%d", i)
					}
					for _, ext := range []string{"arw", "heif"} {
						path := fmt.Sprintf("/DCIM/%s/2025-01-29_21-07-43%s.%s", ext, suffix, ext)
						if want := "A7C00" + num + "." + strings.ToUpper(ext); got[path] != want {
							t.Fatalf("%s is %s, want %s", path, got[path], want)
						}
					}
				}
			}
		})
	}
}

// --- clean tests (with mockFS + fakeTracker) ---

func TestClean(t *testing.T) {
	t.Run("does nothing when Clean=false", func(t *testing.T) {
		sender := &fakeTracker{}
		cli := CLI{opt: Option{Clean: false}, tracker: sender, fs: newMockFS()}
		cli.clean([]string{"/some/dir"})

		if len(sender.getEvents()) != 0 {
			t.Error("should not send any messages when Clean=false")
		}
	})

	t.Run("skips non-existent paths silently", func(t *testing.T) {
		fs := newMockFS()
		fs.statFunc = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		sender := &fakeTracker{}
		cli := CLI{opt: Option{Clean: true}, tracker: sender, fs: fs}
		cli.clean([]string{"/nonexistent"})

		if len(sender.getEvents()) != 0 {
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

		sender := &fakeTracker{}
		cli := CLI{opt: Option{Clean: true}, tracker: sender, fs: osFS{}}
		cli.clean([]string{file})

		if len(sender.getEvents()) != 0 {
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
		// Use real Stat so rename.EmptyDirs works
		fs.statFunc = func(name string) (os.FileInfo, error) { return os.Stat(name) }
		sender := &fakeTracker{}
		cli := CLI{opt: Option{Clean: true}, tracker: sender, fs: fs}
		cli.clean([]string{emptyDir})

		msgs := sender.getEvents()
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message, got %d", len(msgs))
		}
		msg, ok := msgs[0].(cleanedEvent)
		if !ok {
			t.Fatalf("expected cleanedEvent, got %T", msgs[0])
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
		sender := &fakeTracker{}
		cli := CLI{opt: Option{Clean: true}, tracker: sender, fs: fs}
		cli.clean([]string{dir})

		msgs := sender.getEvents()
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message, got %d", len(msgs))
		}
		msg, _ := msgs[0].(cleanedEvent)
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
		sender := &fakeTracker{}
		cli := CLI{opt: Option{Clean: true, Dryrun: true}, tracker: sender, fs: fs}
		cli.clean([]string{emptyDir})

		msgs := sender.getEvents()
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message, got %d", len(msgs))
		}
		msg, _ := msgs[0].(cleanedEvent)
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
		sender := &fakeTracker{}
		cli := CLI{opt: Option{Clean: true}, tracker: sender, fs: fs}
		cli.clean([]string{emptyDir})

		msgs := sender.getEvents()
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message, got %d", len(msgs))
		}
		msg, _ := msgs[0].(cleanedEvent)
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
		sender := &fakeTracker{}
		cli := CLI{opt: Option{Clean: true}, tracker: sender, fs: fs}
		cli.clean([]string{empty1, empty2})

		msgs := sender.getEvents()
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

		sender := &fakeTracker{}
		cli := CLI{opt: Option{Clean: true}, tracker: sender, fs: osFS{}}
		cli.clean([]string{root})

		var got []string
		for _, msg := range sender.getEvents() {
			m, _ := msg.(cleanedEvent)
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

		sender := &fakeTracker{}
		cli := CLI{opt: Option{Clean: true}, tracker: sender, fs: osFS{}}
		cli.clean([]string{root})

		msgs := sender.getEvents()
		if msg, ok := firstClean(msgs); len(msgs) != 1 || !ok || !msg.empty {
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

		sender := &fakeTracker{}
		cli := CLI{
			opt:     Option{Clean: true, Dryrun: true, GroupByDate: true},
			tracker: sender,
			fs:      osFS{},
			paths:   rename.NewReserver(),
			moves:   rename.NewPendingMoves(),
		}
		if _, _, err := cli.rename(photo.Photo{
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
		for _, msg := range sender.getEvents() {
			m, _ := msg.(cleanedEvent)
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

// --- Helpers ---

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

// firstClean is the first message, when it is a cleanedEvent.
func firstClean(msgs []any) (cleanedEvent, bool) {
	if len(msgs) == 0 {
		return cleanedEvent{}, false
	}
	msg, ok := msgs[0].(cleanedEvent)
	return msg, ok
}
