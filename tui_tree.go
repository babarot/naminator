package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The tree view of the interactive screen: the directories as they will be
// after the renames, from the directory all of them are under. A directory
// shows how many photos it will hold, and can be folded (h, l). A long run
// of ordinary files is folded to its ends and "⋯ N more files"; files to
// look at (numbered, without a shooting time) are kept. With --clean, the
// directories left empty are shown as removed.

// tuiItem is a line of the list or of the tree, as the cursor moves on it.
type tuiItem struct {
	kind itemKind
	row  *tuiRow // the photo of a row or a file line
	path string  // a directory's path, or the directory of a file or "more" line
	draw func(selected bool) string
}

type itemKind int

const (
	itemRow  itemKind = iota // a photo in the list
	itemRoot                 // the top of the tree
	itemDir
	itemMore // ⋯ N more files
	itemFile
)

// runFoldAt is the length from which a run of ordinary files is folded.
const runFoldAt = 6

type treeFile struct {
	name    string
	row     *tuiRow
	sidecar bool // a sidecar going with row
}

type treeDir struct {
	name    string
	path    string
	dirs    map[string]*treeDir
	files   []treeFile
	removed bool // removed by --clean
}

func newTreeDir(name, path string) *treeDir {
	return &treeDir{name: name, path: path, dirs: map[string]*treeDir{}}
}

// tree builds the directories as they will be: each photo at its new path
// (or where it is, when it stays), with its sidecars, and the directories
// --clean removes. It returns the top directory.
func (m tuiModel) tree() *treeDir {
	type entry struct {
		path string
		file treeFile
	}
	var entries []entry
	for _, r := range m.rows {
		path := r.key
		if r.usable() && r.target != "" && m.tplErr == "" {
			path = r.target
		}
		entries = append(entries, entry{absPath(path), treeFile{name: filepath.Base(path), row: r}})
		if r.usable() && r.target != "" && !r.same && m.tplErr == "" {
			for _, sc := range m.sidecars[r.photo.Path] {
				to := sidecarTarget(r.photo.Path, r.target, sc)
				entries = append(entries, entry{absPath(to), treeFile{name: filepath.Base(to), row: r, sidecar: true}})
			}
		}
	}
	// the directory everything is under
	var top string
	paths := make([]string, 0, len(entries)+len(m.removed))
	for _, e := range entries {
		paths = append(paths, filepath.Dir(e.path))
	}
	for _, d := range m.removed {
		paths = append(paths, filepath.Dir(absPath(d)))
	}
	for i, p := range paths {
		if i == 0 {
			top = p
			continue
		}
		for top != filepath.Dir(top) && p != top && !strings.HasPrefix(p, top+string(filepath.Separator)) {
			top = filepath.Dir(top)
		}
	}
	root := newTreeDir(m.shortDir(top), top)
	dir := func(path string) *treeDir {
		rel, err := filepath.Rel(top, path)
		if err != nil || rel == "." {
			return root
		}
		d := root
		at := top
		for name := range strings.SplitSeq(rel, string(filepath.Separator)) {
			at = filepath.Join(at, name)
			if d.dirs[name] == nil {
				d.dirs[name] = newTreeDir(name, at)
			}
			d = d.dirs[name]
		}
		return d
	}
	for _, e := range entries {
		d := dir(filepath.Dir(e.path))
		d.files = append(d.files, e.file)
	}
	for _, r := range m.removed {
		dir(absPath(r)).removed = true
	}
	return root
}

// notable reports whether a file is kept when its run is folded.
func (f treeFile) notable() bool {
	return f.row != nil && !f.sidecar && (f.row.suffix != "" || f.row.err != "")
}

// counts returns how many photos a directory will hold, how many of them
// are numbered, and how many are to look at.
func (d *treeDir) counts() (total, numbered, check int) {
	for _, f := range d.files {
		if f.sidecar || f.row == nil {
			continue
		}
		total++
		if f.row.suffix != "" {
			numbered++
		}
		if f.row.skipped() {
			check++
		}
	}
	for _, sub := range d.dirs {
		t, n, c := sub.counts()
		total, numbered, check = total+t, numbered+n, check+c
	}
	return total, numbered, check
}

func (d *treeDir) countText() string {
	if d.removed && len(d.files) == 0 && len(d.dirs) == 0 {
		return Dim.Render("  (removed by -c)")
	}
	total, numbered, check := d.counts()
	s := Dim.Render(fmt.Sprintf("  %d", total))
	if numbered > 0 {
		s += Dim.Render(" · ") + Yellow.Render(fmt.Sprintf("%d numbered", numbered))
	}
	if check > 0 {
		s += Dim.Render(" · ") + Red.Render(fmt.Sprintf("%d to check", check))
	}
	return s
}

// treeItems lays the tree out as lines.
func (m tuiModel) treeItems() []tuiItem {
	root := m.tree()
	lead := func(selected bool) string {
		if selected {
			return Cyan.Render(">") + " "
		}
		return "  "
	}
	rootMark := "▾ "
	if m.folded[root.path] {
		rootMark = "▸ "
	}
	items := []tuiItem{{kind: itemRoot, path: root.path, draw: func(sel bool) string {
		return lead(sel) + Dim.Render(rootMark) + Blue.Render(root.name+"/") + root.countText()
	}}}
	var walk func(d *treeDir, prefix string)
	walk = func(d *treeDir, prefix string) {
		var names []string
		for name := range d.dirs {
			names = append(names, name)
		}
		sort.Strings(names)
		files := append([]treeFile(nil), d.files...)
		sort.SliceStable(files, func(i, j int) bool { return files[i].name < files[j].name })

		// the lines of this directory: its directories, then its files,
		// a long run of ordinary ones folded
		type line struct {
			dir  *treeDir
			file *treeFile
			more int
		}
		var lines []line
		for _, name := range names {
			lines = append(lines, line{dir: d.dirs[name]})
		}
		var run []treeFile
		flush := func() {
			if len(run) >= runFoldAt && !m.expanded[d.path] {
				for i := range run[:2] {
					lines = append(lines, line{file: &run[i]})
				}
				lines = append(lines, line{more: len(run) - 3})
				lines = append(lines, line{file: &run[len(run)-1]})
			} else {
				for i := range run {
					lines = append(lines, line{file: &run[i]})
				}
			}
			run = nil
		}
		for i := range files {
			if files[i].notable() {
				flush()
				lines = append(lines, line{file: &files[i]})
				continue
			}
			run = append(run, files[i])
		}
		flush()

		for i, l := range lines {
			branch, next := "├─ ", prefix+"│  "
			if i == len(lines)-1 {
				branch, next = "└─ ", prefix+"   "
			}
			switch {
			case l.dir != nil:
				sub := l.dir
				folded := m.folded[sub.path]
				mark := "▾ "
				if folded {
					mark = "▸ "
				}
				empty := sub.removed && len(sub.files) == 0 && len(sub.dirs) == 0
				items = append(items, tuiItem{kind: itemDir, path: sub.path, draw: func(sel bool) string {
					if empty {
						return lead(sel) + Dim.Render(prefix+branch+"  "+sub.name+"/") + sub.countText()
					}
					return lead(sel) + Dim.Render(prefix+branch+mark) + Blue.Render(sub.name+"/") + sub.countText()
				}})
				if !folded {
					walk(sub, next)
				}
			case l.more > 0:
				n := l.more
				items = append(items, tuiItem{kind: itemMore, path: d.path, draw: func(sel bool) string {
					return lead(sel) + Dim.Render(prefix+branch) + Dim.Render(fmt.Sprintf("⋯ %d more files", n))
				}})
			default:
				f := *l.file
				items = append(items, tuiItem{kind: itemFile, row: f.row, path: d.path, draw: func(sel bool) string {
					return lead(sel) + Dim.Render(prefix+branch) + m.fileText(f)
				}})
			}
		}
	}
	// The top folds too; l on it opens everything again
	if !m.folded[root.path] {
		walk(root, "")
	}
	return items
}

// fileText draws a file of the tree.
func (m tuiModel) fileText(f treeFile) string {
	r := f.row
	switch {
	case f.sidecar:
		return Dim.Render(f.name)
	case !r.read:
		return Dim.Render(f.name + "  (reading EXIF…)")
	case r.skipped():
		return Red.Render(f.name) + Dim.Render("  (no shooting time)")
	case r.same || m.tplErr != "":
		return Dim.Render(f.name + "  (already named)")
	}
	s := f.name + Dim.Render("  ← "+r.key)
	if r.suffix != "" {
		s += "  " + Yellow.Render(r.suffix)
	}
	if r.fileTime {
		s += "  " + Yellow.Render(iconFileTime) + Dim.Render(" file time")
	}
	return s
}

// predictRemoved returns the directories --clean would remove after the
// renames planned: those of the arguments left empty, as a dry run
// predicts them.
func (m tuiModel) predictRemoved() []string {
	moves := newPendingMoves()
	for _, r := range m.rows {
		if !r.usable() || r.same || r.target == "" {
			continue
		}
		moves.add(r.photo.Path, r.target)
		for _, sc := range m.sidecars[r.photo.Path] {
			moves.add(sc, sidecarTarget(r.photo.Path, r.target, sc))
		}
	}
	var removed []string
	for _, arg := range m.args {
		if fi, err := os.Stat(arg); err != nil || !fi.IsDir() {
			continue
		}
		if _, dirs, err := emptyDirs(arg, moves); err == nil {
			removed = append(removed, dirs...)
		}
	}
	return removed
}

// fold folds or opens the tree under the cursor: l opens a folded
// directory, enters an open one, or shows the files a "more" line hides; h
// folds an open directory, or goes up to the directory above. On the top
// line, h folds and l opens every directory.
func (m *tuiModel) fold(open bool) {
	items := m.items()
	if m.view != viewTree || m.sel >= len(items) {
		return
	}
	it := items[m.sel]
	switch {
	case it.kind == itemRoot && open:
		// open everything, the top and what a format before folded included
		clear(m.folded)
	case it.kind == itemRoot:
		// fold the directories under the top; when there are none left to
		// fold, the top itself
		folded := false
		for _, sub := range items {
			if sub.kind == itemDir && !m.folded[sub.path] {
				m.folded[sub.path] = true
				folded = true
			}
		}
		if !folded {
			m.folded[it.path] = true
		}
	case open && it.kind == itemDir:
		if m.folded[it.path] {
			delete(m.folded, it.path)
		} else {
			m.sel++
		}
	case open && it.kind == itemMore:
		m.expanded[it.path] = true
	case !open && it.kind == itemDir && !m.folded[it.path]:
		m.folded[it.path] = true
	case !open:
		parent := it.path
		if it.kind == itemDir {
			parent = filepath.Dir(it.path)
		}
		if it.kind != itemDir && m.expanded[it.path] {
			delete(m.expanded, it.path)
		}
		for i, x := range m.items() {
			if (x.kind == itemDir || x.kind == itemRoot) && x.path == parent {
				m.sel = i
				break
			}
		}
	}
	m.sel = min(m.sel, len(m.items())-1)
}
