package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/babarot/naminator/internal/photo"
	"github.com/babarot/naminator/internal/rename"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// A phase holds the rows of one phase of the run and draws them. Both
// displays use it: the terminal one (model) and the plain one (plainSender).

type itemStatus int

const (
	itemQueued itemStatus = iota
	itemRunning
	itemDone
	itemCanceled
)

// mark is how a finished row shows its result.
type mark int

const (
	markNone   mark = iota // ✓ with the name dim: nothing to show (read, already named, kept)
	markDone               // ✓: renamed
	markChange             // ~ in yellow: would be renamed (dry run)
	markDelete             // - in red: a directory removed
	markFailed             // ✗ in red
)

type item struct {
	key    string
	name   string
	status itemStatus
	mark   mark
	label  string
	err    string
	begin  time.Time
	end    time.Time
	// took is what the work took, shown in the elapsed column of the rows
	// that have one (reading EXIF).
	took time.Duration
	// printed is set once the row is printed above the live view.
	printed bool
}

type phase struct {
	phaseStartMsg
	items    []*item
	byKey    map[string]*item
	finished []*item // in the order they finished
	start    time.Time
	last     time.Time // when the last row finished
	// compact is set when the rows do not fit and only the running ones are
	// drawn, the finished ones printed above. Once set, it stays set.
	compact  bool
	canceled bool
	// peak is the most lines the compact view has had above its blank line
	// and progress bar. The view keeps that height, so the bar stays put
	// while the last photos are read and the running rows go away.
	peak int
}

// Failure is an error to report in full at the end of the run.
type Failure struct {
	Name string
	Err  string
}

func newPhase(msg phaseStartMsg, now time.Time) *phase {
	p := &phase{phaseStartMsg: msg, byKey: map[string]*item{}, start: now, last: now}
	for _, key := range msg.keys {
		p.add(key, filepath.Base(key))
	}
	return p
}

func (p *phase) add(key, name string) *item {
	it := &item{key: key, name: name}
	p.items = append(p.items, it)
	p.byKey[key] = it
	return it
}

// apply records a message about a row, and returns the row when it finished
// and the failure to report, if any.
func (p *phase) apply(msg tea.Msg, now time.Time) (*item, *Failure) {
	switch msg := msg.(type) {
	case exifStartMsg:
		if it := p.byKey[msg.image]; it != nil && it.status == itemQueued {
			it.status = itemRunning
			it.begin = now
		}
		return nil, nil

	case exifResultMsg:
		key := msg.image
		if key == "" {
			key = msg.photo.Path
		}
		it := p.byKey[key]
		if it == nil {
			it = p.add(key, filepath.Base(key))
		}
		if msg.err != nil {
			return p.finish(it, markFailed, "", msg.err.Error(), msg.duration, now)
		}
		return p.finish(it, markNone, "", "", msg.duration, now)

	case renameResultMsg:
		it := p.byKey[msg.photo.Path]
		if it == nil {
			it = p.add(msg.photo.Path, msg.photo.Name)
		}
		switch {
		case msg.err != nil:
			return p.finish(it, markFailed, "", msg.err.Error(), 0, now)
		case msg.photo.RenamedPath == msg.photo.Path:
			return p.finish(it, markNone, "already named", "", 0, now)
		case msg.dryrun:
			return p.finish(it, markChange, "→ "+p.target(msg.photo.RenamedPath)+sidecarNote(msg.photo), "", 0, now)
		default:
			return p.finish(it, markDone, "→ "+p.target(msg.photo.RenamedPath)+sidecarNote(msg.photo), "", 0, now)
		}

	case warnMsg:
		return nil, &Failure{Name: msg.name, Err: msg.text}

	case cleanResultMsg:
		it := p.byKey[msg.dir]
		if it == nil {
			it = p.add(msg.dir, msg.dir)
		}
		switch {
		case msg.err != nil:
			return p.finish(it, markFailed, "", msg.err.Error(), 0, now)
		case !msg.empty:
			return p.finish(it, markNone, "kept (not empty)", "", 0, now)
		case msg.dryrun:
			return p.finish(it, markDelete, "would remove (empty)", "", 0, now)
		default:
			return p.finish(it, markDelete, "removed (empty)", "", 0, now)
		}
	}
	return nil, nil
}

// sidecarNote names the sidecars that went with a photo, as "  + xmp".
func sidecarNote(photo photo.Photo) string {
	var exts []string
	for _, sc := range photo.RenamedSidecars {
		exts = append(exts, strings.ToLower(strings.TrimPrefix(filepath.Ext(sc), ".")))
	}
	if len(exts) == 0 {
		return ""
	}
	return "  + " + strings.Join(exts, " ")
}

func (p *phase) finish(it *item, m mark, label, err string, took time.Duration, now time.Time) (*item, *Failure) {
	if it.status == itemDone || it.status == itemCanceled {
		// A result after Ctrl+C, of work that was already running.
		return nil, nil
	}
	if it.status == itemQueued {
		it.begin = now
	}
	it.status, it.mark, it.label, it.err, it.took, it.end = itemDone, m, label, err, took, now
	p.finished = append(p.finished, it)
	p.last = now
	if m == markFailed {
		return it, &Failure{Name: it.name, Err: err}
	}
	return it, nil
}

// cancel shows the running rows as canceled; those not started stay queued
// and are left out.
func (p *phase) cancel(now time.Time) {
	p.canceled = true
	for _, it := range p.items {
		if it.status == itemRunning {
			it.status = itemCanceled
			it.end = now
		}
	}
}

// target is how a rename target is shown: from base when it is under it, else
// from the home directory.
func (p *phase) target(path string) string {
	if p.base != "" {
		if rel, err := filepath.Rel(rename.AbsPath(p.base), rename.AbsPath(path)); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return rel
		}
	}
	return abbrevHome(path, os.Getenv("HOME"))
}

func (p *phase) count(m mark) int {
	n := 0
	for _, it := range p.finished {
		if it.mark == m {
			n++
		}
	}
	return n
}

func (p *phase) countStatus(s itemStatus) int {
	n := 0
	for _, it := range p.items {
		if it.status == s {
			n++
		}
	}
	return n
}

// unit names what the phase counts, as "photos" in "58/240 photos".
func (p *phase) unit() string {
	if p.kind == phaseClean {
		return "directories"
	}
	return "photos"
}

func (p *phase) title() string {
	n := len(p.items)
	switch p.kind {
	case phaseExif:
		return fmt.Sprintf("Reading EXIF of %d %s", n, plural(n, "photo", "photos"))
	case phaseRename:
		where := ""
		if p.base != "" {
			prep := "in"
			if p.into {
				prep = "into"
			}
			where = fmt.Sprintf(" %s %s", prep, abbrevHome(rename.AbsPath(p.base), os.Getenv("HOME")))
		}
		if p.dry {
			return fmt.Sprintf("Planning %d %s%s (dry run)", n, plural(n, "rename", "renames"), where)
		}
		return fmt.Sprintf("Renaming %d %s%s", n, plural(n, "photo", "photos"), where)
	default:
		if p.dry {
			return "Removing empty directories (dry run)"
		}
		return "Removing empty directories"
	}
}

// outcome replaces the heading once the phase is over, as in
// "✓ Read EXIF of 240 photos in 10.2s" or "✗ ..., 2 failed".
func (p *phase) outcome() string {
	if p.canceled {
		return Dim.Render(iconError+" "+p.title()) + Dim.Render(" (canceled)")
	}
	took := formatSeconds(p.last.Sub(p.start))
	failed := p.count(markFailed)
	var line string
	switch p.kind {
	case phaseExif:
		n := len(p.items)
		line = fmt.Sprintf("Read EXIF of %d %s in %s", n, plural(n, "photo", "photos"), took)
	case phaseRename:
		if p.dry {
			n := p.count(markChange)
			line = fmt.Sprintf("Would rename %d %s (dry run)", n, plural(n, "photo", "photos"))
		} else {
			n := p.count(markDone)
			line = fmt.Sprintf("Renamed %d %s in %s", n, plural(n, "photo", "photos"), took)
		}
		if n := p.count(markNone); n > 0 {
			line += fmt.Sprintf(", %d already named", n)
		}
	default:
		n := p.count(markDelete)
		switch {
		case n == 0:
			line = "No empty directories to remove"
		case p.dry:
			line = fmt.Sprintf("Would remove %d empty %s", n, plural(n, "directory", "directories"))
		default:
			line = fmt.Sprintf("Removed %d empty %s in %s", n, plural(n, "directory", "directories"), took)
		}
		if k := p.count(markNone); k > 0 {
			line += fmt.Sprintf(", %d kept", k)
		}
	}
	if failed > 0 {
		return Red.Render(iconError) + " " + line + ", " + Red.Render(fmt.Sprintf("%d failed", failed))
	}
	return Green.Render(iconSuccess) + " " + line
}

// noneWord names the rows with nothing to show when they are only counted.
func (p *phase) noneWord() string {
	switch p.kind {
	case phaseExif:
		return "read"
	case phaseRename:
		return "already named"
	default:
		return "kept"
	}
}

// shownAbove reports whether a finished row is printed above the compact
// view; the rows with nothing to show are only counted.
func shownAbove(it *item) bool {
	return it.mark != markNone
}

// timed reports whether the rows have an elapsed column.
func (p *phase) timed() bool {
	return p.kind == phaseExif
}

const (
	maxNameWidth     = 24
	elapsedWidth     = 5
	progressBarWidth = 24
)

// nameWidth is the width the names are padded to, for aligning the rows.
func (p *phase) nameWidth() int {
	w := 0
	for _, it := range p.items {
		w = max(w, ansi.StringWidth(it.name))
	}
	return min(w, maxNameWidth)
}

// row draws one row. spin is the spinner's frame for a running row; width
// cuts the row to the terminal (0 for no limit).
func (p *phase) row(it *item, spin string, now time.Time, width int) string {
	name := padName(it.name, p.nameWidth())
	elapsed := func(d time.Duration) string {
		if !p.timed() {
			return ""
		}
		return Dim.Render(formatElapsed(d)) + "  "
	}
	var line string
	switch it.status {
	case itemQueued:
		gap := ""
		if p.timed() {
			gap = strings.Repeat(" ", elapsedWidth+2)
		}
		line = fmt.Sprintf("  %s %s  %s%s", Dim.Render(iconQueued), Dim.Render(name), gap, Dim.Render("queued"))
	case itemRunning:
		status := "exiftool"
		if p.kind != phaseExif {
			status = "working"
		}
		line = fmt.Sprintf("  %s %s  %s%s", spin, name, elapsed(now.Sub(it.begin)), Dim.Render(status))
	case itemCanceled:
		line = fmt.Sprintf("  %s %s", Dim.Render(iconError), Dim.Render(strings.TrimRight(name, " ")+" (canceled)"))
	default:
		switch it.mark {
		case markFailed:
			first, _, more := strings.Cut(it.err, "\n")
			if more {
				first += iconEllipsis
			}
			line = fmt.Sprintf("  %s %s  %s%s", Red.Render(iconError), Bold.Render(name), elapsed(it.took), Dim.Render(first))
		case markNone:
			line = fmt.Sprintf("  %s %s  %s%s", Green.Render(iconSuccess), Dim.Render(name), elapsed(it.took), Dim.Render(it.label))
		case markDelete:
			line = fmt.Sprintf("  %s %s  %s", Red.Render(iconRemove), name, Red.Render(it.label))
		default:
			icon := Green.Render(iconSuccess)
			if it.mark == markChange {
				icon = Yellow.Render(iconChange)
			}
			label := it.label
			if rest, ok := strings.CutPrefix(label, "→ "); ok {
				label = Dim.Render("→") + " " + rest
			}
			line = fmt.Sprintf("  %s %s  %s%s", icon, name, elapsed(it.took), label)
		}
	}
	return fitLine(line, width)
}

// padName cuts name to w cells with "…", or pads it to w.
func padName(name string, w int) string {
	if ansi.StringWidth(name) > w {
		// A wide character that does not fit leaves a cell short
		name = ansi.Truncate(name, w, iconEllipsis)
	}
	return name + strings.Repeat(" ", max(0, w-ansi.StringWidth(name)))
}

// fitLine drops trailing spaces and cuts line to width cells with "…".
func fitLine(line string, width int) string {
	line = strings.TrimRight(line, " ")
	if width > 0 && ansi.StringWidth(line) > width {
		return ansi.Truncate(line, width, iconEllipsis)
	}
	return line
}

// progress returns how many rows have finished, of how many.
func (p *phase) progress() (done, total int) {
	return len(p.finished), len(p.items)
}

// gauge is how many exiftool processes run, of the limit: "exiftool 8/8",
// in yellow at the limit. Only while reading EXIF.
func (p *phase) gauge() []string {
	if p.kind != phaseExif || p.limit == 0 {
		return nil
	}
	running := p.countStatus(itemRunning)
	g := fmt.Sprintf("exiftool %d/%d", running, p.limit)
	if running >= p.limit {
		return []string{Yellow.Render(g)}
	}
	return []string{Dim.Render(g)}
}

func joinDim(parts []string) string {
	return strings.Join(parts, Dim.Render(" · "))
}

// heading is the line above the rows of the full view: the title and the
// progress while the phase runs.
func (p *phase) heading(now time.Time, width int) string {
	done, total := p.progress()
	line := p.title()
	if p.kind != phaseClean {
		line += fmt.Sprintf("  %s  %d/%d %s", progressBar(done, total, progressBarWidth), done, total, p.unit())
	}
	line += "  " + Dim.Render(formatSeconds(now.Sub(p.start)))
	more := p.gauge()
	if q := p.countStatus(itemQueued); q > 0 && p.kind == phaseExif {
		more = append(more, Dim.Render(fmt.Sprintf("%d queued", q)))
	}
	if len(more) > 0 {
		line += Dim.Render("  · ") + joinDim(more)
	}
	return fitLine(line, width)
}

// fullView draws the heading and every row, while they fit the terminal.
// Once the phase is over, the heading is its outcome and the hint is gone.
func (p *phase) fullView(over bool, spin string, now time.Time, width int) []string {
	var lines []string
	if over {
		lines = append(lines, p.outcome(), "")
	} else {
		lines = append(lines, p.heading(now, width), "")
	}
	for _, it := range p.items {
		if over && it.status == itemQueued {
			continue
		}
		lines = append(lines, p.row(it, spin, now, width))
	}
	if !over {
		lines = append(lines, "", "  "+Dim.Render("ctrl+c to cancel"))
	}
	return lines
}

const (
	// compactAt is the number of rows from which the view is compact.
	compactAt = 11
	// maxRunningRows is how many running rows the compact view shows at most.
	maxRunningRows = 5
)

// wantCompact reports whether the rows should be drawn compact: many rows,
// or a full view taller than the terminal.
func (p *phase) wantCompact(height int) bool {
	if len(p.items) >= compactAt {
		return true
	}
	// A frame takes a line more than its lines (the cursor's).
	return height > 0 && len(p.fullView(false, "", p.start, 0))+1 > height
}

// compactView draws the running rows, what is left, and the progress. The
// title and the finished rows are printed above it. Rows that went away
// leave blank lines, down to the height the view has had (peak).
func (p *phase) compactView(spin string, now time.Time, width, height int) []string {
	var running []*item
	for _, it := range p.items {
		if it.status == itemRunning {
			running = append(running, it)
		}
	}
	// lines besides the running rows: what is left, the blank line, the
	// progress, and the line under the frame
	fixed := 4
	tight := height > 0 && height < fixed+1
	if tight {
		fixed--
	}
	shown := min(len(running), maxRunningRows)
	if height > 0 {
		shown = min(len(running), max(1, min(maxRunningRows, height-fixed)))
	}
	var lines []string
	for _, it := range running[:shown] {
		lines = append(lines, p.row(it, spin, now, width))
	}
	var left []string
	if more := len(running) - shown; more > 0 {
		left = append(left, fmt.Sprintf("+%d more", more))
	}
	if q := p.countStatus(itemQueued); q > 0 {
		left = append(left, fmt.Sprintf("%d queued", q))
	}
	if n := p.count(markNone); n > 0 {
		left = append(left, fmt.Sprintf("%d %s", n, p.noneWord()))
	}
	if len(left) > 0 {
		lines = append(lines, fitLine("  "+Dim.Render(strings.Join(left, " · ")), width))
	}
	p.peak = max(p.peak, len(lines))
	keep := p.peak
	if height > 0 {
		// within the terminal: the blank line, the progress, the cursor's line
		keep = min(keep, height-fixed+1)
	}
	for len(lines) < keep {
		lines = append(lines, "")
	}
	if !tight {
		lines = append(lines, "")
	}
	done, total := p.progress()
	line := fmt.Sprintf("  %s  %d/%d %s  %s", progressBar(done, total, progressBarWidth), done, total, p.unit(), Dim.Render(formatSeconds(now.Sub(p.start))))
	more := append(p.gauge(), Dim.Render("ctrl+c to cancel"))
	lines = append(lines, fitLine(line+Dim.Render("  · ")+joinDim(more), width))
	return lines
}

// compactEnd is what the compact view leaves once the phase is over: the
// rows only counted, and the outcome.
func (p *phase) compactEnd() []string {
	var lines []string
	if n := p.count(markNone); n > 0 {
		lines = append(lines, fmt.Sprintf("  %s %s", Green.Render(iconSuccess), Dim.Render(fmt.Sprintf("%d %s", n, p.noneWord()))))
	}
	return append(lines, "", p.outcome())
}

// errorReport prints each failure in full, as gh-infra's PrintErrors, each
// followed by a blank line:
//
//	broken.jpg:
//	  exiftool: Error: File format error
func errorReport(failures []Failure) []string {
	var lines []string
	for _, f := range failures {
		lines = append(lines, "  "+Bold.Render(f.Name+":"))
		for l := range strings.SplitSeq(f.Err, "\n") {
			lines = append(lines, "    "+Dim.Render(l))
		}
		lines = append(lines, "")
	}
	return lines
}

// progressBar draws done of total as a bar of width cells.
func progressBar(done, total, width int) string {
	filled := 0
	if total > 0 {
		filled = min(width, done*width/total)
	}
	return Cyan.Render(strings.Repeat("━", filled)) + Dim.Render(strings.Repeat("─", width-filled))
}

// formatSeconds formats d as in "2.1s".
func formatSeconds(d time.Duration) string {
	return fmt.Sprintf("%.1fs", max(d, 0).Seconds())
}

// formatElapsed formats d for the elapsed column, right-aligned.
func formatElapsed(d time.Duration) string {
	return fmt.Sprintf("%*.1fs", elapsedWidth-1, max(d, 0).Seconds())
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
