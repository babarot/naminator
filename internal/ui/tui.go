package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"

	"github.com/babarot/naminator/internal/config"
	"github.com/babarot/naminator/internal/photo"
	"github.com/babarot/naminator/internal/rename"
	"github.com/babarot/naminator/internal/ui/keymap"
	"github.com/babarot/naminator/internal/ui/theme"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

// The interactive screen (naminator -i): it reads the photos' EXIF data,
// shows the path each one would get with the format, and lets the format be
// edited or a preset chosen while the paths follow. Once confirmed, the run
// goes on with the usual display, from the rename phase.
//
// It has two modes, as vim: normal, where the keys move in the list, and
// insert (i), where they edit the format. Enter in insert mode only leaves
// it, so that a rename never starts while typing.

// Result is how the screen ended.
type Result int

const (
	tuiRunning Result = iota
	Confirmed         // rename with the format
	Quit              // q: nothing renamed
	Canceled          // Ctrl+C or a signal: nothing renamed
)

// Decision is what the screen decided.
type Decision struct {
	Result Result
	// Format is the format to rename with, under Base, when confirmed.
	Format string
	Base   string
	// Photos are the photos to rename, in the order they were taken, those
	// given the file's time included.
	Photos []photo.Photo
	// Targets are the paths previewed, by photo path: the run gives them,
	// so that the photos go where they were shown to go.
	Targets map[string]string
	// Failed are the photos whose EXIF data could not be read, to report.
	Failed []Failure
}

type tuiView int

const (
	viewList tuiView = iota
	viewTree
)

func (v tuiView) String() string {
	if v == viewTree {
		return "tree"
	}
	return "list"
}

// tuiRow is one photo of the list, in the order the photos were given.
type tuiRow struct {
	key   string // the path the photo was read from
	photo photo.Photo
	read  bool   // its EXIF data has come
	err   string // reading it failed (no shooting time, ...)
	// target is where it would go with the format, suffix the "_1" the
	// reserver added to it, and same reports that it is already named so
	target string
	suffix string
	same   bool
	// fileTime uses the file's modification time for a photo without a
	// shooting time (m), instead of skipping it
	fileTime bool
}

// usable reports whether the photo gets a path: read, with a shooting
// time or the file's time.
func (r *tuiRow) usable() bool {
	return r.read && (r.err == "" || r.fileTime)
}

// skipped reports whether the photo is left out: read without a shooting
// time, and not given the file's time.
func (r *tuiRow) skipped() bool {
	return r.read && r.err != "" && !r.fileTime
}

type tuiModel struct {
	args    []string
	destDir string
	dry     bool
	fs      rename.FileSystem
	cfgPath string
	presets []rename.Preset
	cancel  func()
	keys    keymap.Map

	rows   []*tuiRow
	byKey  map[string]*tuiRow
	nRead  int
	format string
	cursor int // in runes of format
	namer  *rename.Namer
	tplErr string
	// sidecars are found once every photo is read
	sidecars map[string][]string

	// view is the list or the tree; folded are the directories folded in
	// the tree, and expanded those whose runs of files are all shown
	view     tuiView
	folded   map[string]bool
	expanded map[string]bool
	// clean is --clean, and removed the directories it would remove
	clean   bool
	removed []string

	// st are the styles, for a dark background until the terminal tells its
	// own
	st theme.Styles

	insert  bool
	confirm bool
	sel     int
	top     int
	notice  string
	width   int
	height  int
	spinner spinner.Model
	result  Result
}

func newTUIModel(args, images []string, format, destDir string, dry, clean bool, presets []rename.Preset, fsys rename.FileSystem, cfgPath string, cancel func()) tuiModel {
	width, height := 100, 30
	if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		width, height = w, h
	}
	m := tuiModel{
		args: args, destDir: destDir, dry: dry, clean: clean, fs: fsys, cfgPath: cfgPath, presets: presets, cancel: cancel, keys: keymap.Default(), st: theme.For(true),
		byKey:    map[string]*tuiRow{},
		folded:   map[string]bool{},
		expanded: map[string]bool{},
		width:    width,
		height:   height,
		spinner:  spinner.New(spinner.WithSpinner(spinner.Jump), spinner.WithStyle(theme.For(true).Active)),
	}
	for _, img := range images {
		r := &tuiRow{key: img}
		m.rows = append(m.rows, r)
		m.byKey[img] = r
	}
	m.setFormat(format)
	return m
}

func (m tuiModel) Init() tea.Cmd { return tea.Batch(m.spinner.Tick, tea.RequestBackgroundColor) }

// setFormat sets the format, puts the cursor at its end, and plans again.
func (m *tuiModel) setFormat(f string) {
	m.format = f
	m.cursor = len([]rune(f))
	m.plan()
}

// plan gives each read photo its path with the format: in the order the
// photos are taken, through a reserver, as the run will.
func (m *tuiModel) plan() {
	tpl, err := rename.ParseTemplate(m.format)
	if err != nil {
		m.tplErr, m.namer = err.Error(), nil
		return
	}
	m.tplErr = ""
	m.removed = nil
	m.namer = rename.NewNamer(tpl, m.args, m.destDir)
	var photos []photo.Photo
	for _, r := range m.rows {
		if r.usable() {
			photos = append(photos, r.photo)
		}
	}
	photo.SortByTime(photos)
	reserver := rename.NewReserver()
	for _, p := range photos {
		r := m.byKey[m.keyOf(p)]
		want := m.namer.Target(p)
		r.target = reserver.Reserve(m.fs, p.Path, want)
		r.same = r.target == p.Path
		r.suffix = strings.TrimPrefix(strings.TrimSuffix(r.target, filepath.Ext(r.target)), strings.TrimSuffix(want, filepath.Ext(want)))
	}
	m.removed = nil
	if m.clean && m.nRead == len(m.rows) {
		m.removed = m.predictRemoved()
	}
}

// keyOf returns the row key of a photo read: the path it was read from.
func (m tuiModel) keyOf(p photo.Photo) string {
	if _, ok := m.byKey[p.Path]; ok {
		return p.Path
	}
	for k, r := range m.byKey {
		if r.read && r.photo.Path == p.Path {
			return k
		}
	}
	return p.Path
}

// photos returns the photos read with a shooting time, for the run.
func (m tuiModel) photos() []photo.Photo {
	var out []photo.Photo
	for _, r := range m.rows {
		if r.usable() {
			out = append(out, r.photo)
		}
	}
	return out
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	if tm, ok := next.(tuiModel); ok {
		// The lines can be fewer after a photo is read or the format
		// changes: keep the cursor on one of them
		tm.sel = max(0, min(tm.sel, len(tm.items())-1))
		// keep the lines shown, moving them only to keep the cursor in
		tm.top = tm.window().top
		return tm, cmd
	}
	return next, cmd
}

func (m tuiModel) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.result != tuiRunning {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.BackgroundColorMsg:
		m.st = theme.For(msg.IsDark())
		m.spinner.Style = m.st.Active
		return m, nil

	case exifResultMsg:
		r := m.byKey[msg.image]
		if r == nil || r.read {
			return m, nil
		}
		r.read, r.photo = true, msg.photo
		if msg.err != nil {
			r.err = msg.err.Error()
		}
		m.nRead++
		if m.nRead == len(m.rows) {
			m.sidecars, _ = rename.FindSidecars(m.fs, m.photos())
		}
		m.plan()
		return m, nil

	case interruptMsg:
		return m.end(Canceled)

	case tea.KeyPressMsg:
		return m.key(msg)

	case tea.MouseMsg:
		return m.mouse(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

// end closes the screen with r. Unless confirmed, the EXIF reading stops.
// What was decided is read from the final model (decision), and the run
// is another program.
func (m tuiModel) end(r Result) (tea.Model, tea.Cmd) {
	m.result = r
	if r != Confirmed && m.cancel != nil {
		m.cancel()
	}
	return m, tea.Quit
}

// decision is what the screen decided.
func (m tuiModel) decision() Decision {
	d := Decision{Result: m.result, Format: m.format, Base: m.base(), Photos: m.photos(), Targets: map[string]string{}}
	photo.SortByTime(d.Photos)
	for _, row := range m.rows {
		if row.usable() && row.target != "" {
			d.Targets[row.photo.Path] = row.target
		}
		if row.skipped() {
			d.Failed = append(d.Failed, Failure{Name: filepath.Base(row.key), Err: row.err})
		}
	}
	return d
}

// ready reports whether the photos can be renamed, and why not.
func (m tuiModel) ready() (bool, string) {
	switch {
	case m.tooLow():
		return false, "the terminal is too low"
	case m.nRead < len(m.rows):
		return false, "after EXIF"
	case m.tplErr != "":
		return false, "fix the format"
	case m.countMoves() == 0:
		return false, "nothing to rename"
	}
	return true, ""
}

// key handles a key: what it does is looked up for the state the screen is
// in, so that a key never acts on what is under a question.
func (m tuiModel) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.notice = ""
	state := m.uiState()
	a := m.keys.Action(msg.String(), state.scopes()...)
	if a == keymap.Interrupt {
		return m.end(Canceled)
	}
	switch state {
	case stateTooLow:
		return m.tooLowKey(a)
	case stateConfirm:
		return m.confirmKey(a)
	case stateEdit:
		return m.editKey(a, msg)
	case stateList, stateTree:
		return m.listKey(a)
	}
	return m, nil
}

// tooLowKey handles a key when the screen is not drawn: nothing that cannot
// be seen is done.
func (m tuiModel) tooLowKey(a keymap.Action) (tea.Model, tea.Cmd) {
	if a == keymap.Quit {
		return m.end(Quit)
	}
	return m, nil
}

// confirmKey handles the answer to y/N: anything but y is no.
func (m tuiModel) confirmKey(a keymap.Action) (tea.Model, tea.Cmd) {
	m.confirm = false
	if a == keymap.Yes {
		return m.end(Confirmed)
	}
	return m, nil
}

// screenKey handles the keys that work both in the list and the tree and
// while the format is edited, and reports whether it was one of them.
func (m *tuiModel) screenKey(a keymap.Action) bool {
	switch a {
	case keymap.NextPreset, keymap.PrevPreset:
		m.cyclePreset(a == keymap.NextPreset)
	case keymap.Up:
		m.sel = max(0, m.sel-1)
	case keymap.Down:
		m.sel = min(len(m.items())-1, m.sel+1)
	case keymap.PageUp:
		m.page(-1)
	case keymap.PageDown:
		m.page(1)
	case keymap.ToggleView:
		m.toggleView()
	case keymap.SaveFormat:
		m.save()
	default:
		return false
	}
	return true
}

// editKey handles a key while the format is edited: one that is not bound
// is typed.
func (m tuiModel) editKey(a keymap.Action, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.screenKey(a) {
		return m, nil
	}
	r := []rune(m.format)
	switch a {
	case keymap.Done:
		m.insert = false
	case keymap.CursorLeft:
		m.cursor = max(0, m.cursor-1)
	case keymap.CursorRight:
		m.cursor = min(len(r), m.cursor+1)
	case keymap.CursorStart:
		m.cursor = 0
	case keymap.CursorEnd:
		m.cursor = len(r)
	case keymap.DeleteBack:
		if m.cursor > 0 {
			m.format = string(r[:m.cursor-1]) + string(r[m.cursor:])
			m.cursor--
			m.plan()
		}
	case keymap.None:
		if t := msg.Text; t != "" && msg.Mod == 0 || msg.Mod == tea.ModShift && t != "" {
			m.format = string(r[:m.cursor]) + t + string(r[m.cursor:])
			m.cursor += len([]rune(t))
			m.plan()
		}
	}
	return m, nil
}

// listKey handles a key in the list or the tree.
func (m tuiModel) listKey(a keymap.Action) (tea.Model, tea.Cmd) {
	if m.screenKey(a) {
		return m, nil
	}
	switch a {
	case keymap.Edit:
		m.insert = true
	case keymap.Append:
		m.insert, m.cursor = true, len([]rune(m.format))
	case keymap.Top:
		m.sel = 0
	case keymap.Bottom:
		m.sel = len(m.items()) - 1
	case keymap.Fold:
		m.fold(false)
	case keymap.Unfold:
		m.fold(true)
	case keymap.FileTime:
		m.toggleFileTime()
	case keymap.Rename:
		if ok, _ := m.ready(); ok {
			m.confirm = true
		}
	case keymap.Quit:
		return m.end(Quit)
	}
	return m, nil
}

// toggleFileTime gives the photo under the cursor, read without a shooting
// time, the file's modification time instead, or takes it back.
func (m *tuiModel) toggleFileTime() {
	r := m.selected()
	if r == nil || !r.read || r.err == "" {
		return
	}
	if r.fileTime {
		r.fileTime = false
	} else {
		fi, err := os.Stat(r.key)
		if err != nil {
			m.notice = m.st.Failure.Render("no file time: " + err.Error())
			return
		}
		r.fileTime = true
		r.photo = photo.Photo{
			Name:      filepath.Base(r.key),
			Path:      r.key,
			Dir:       filepath.Dir(r.key),
			Extension: strings.ToLower(strings.TrimPrefix(filepath.Ext(r.key), ".")),
			CreatedAt: fi.ModTime(),
		}
	}
	if m.nRead == len(m.rows) {
		m.sidecars, _ = rename.FindSidecars(m.fs, m.photos())
	}
	m.plan()
}

// toggleView switches between the list and the tree.
func (m *tuiModel) toggleView() {
	if m.view == viewList {
		m.view = viewTree
	} else {
		m.view = viewList
	}
	m.sel = 0
}

// items returns the lines of the view, as the cursor moves on them.
func (m tuiModel) items() []tuiItem {
	if m.view == viewTree {
		return m.treeItems()
	}
	fromW := 0
	for _, r := range m.rows {
		fromW = max(fromW, ansi.StringWidth(m.shown(r, r.key)))
	}
	fromW = min(fromW, 36)
	items := make([]tuiItem, len(m.rows))
	for i, r := range m.rows {
		items[i] = tuiItem{kind: itemRow, row: r, draw: func(sel bool) string { return m.row(r, sel, fromW) }}
	}
	return items
}

// selected returns the photo under the cursor, if a photo is there.
func (m tuiModel) selected() *tuiRow {
	items := m.items()
	if m.sel < len(items) {
		return items[m.sel].row
	}
	return nil
}

// cyclePreset sets the next preset, or the previous one; from a format of
// one's own, the first.
func (m *tuiModel) cyclePreset(next bool) {
	i := -1
	for j, p := range m.presets {
		if p.Format == m.format {
			i = j
		}
	}
	switch {
	case i < 0:
		i = 0
	case next:
		i = (i + 1) % len(m.presets)
	default:
		i = (i + len(m.presets) - 1) % len(m.presets)
	}
	m.setFormat(m.presets[i].Format)
}

// presetName names the format when it is a preset.
func (m tuiModel) presetName() string {
	for _, p := range m.presets {
		if p.Format == m.format {
			return p.Name
		}
	}
	return "custom"
}

// presetLabel is presetName with the flags the preset stands for, as
// "by date (-t)".
func (m tuiModel) presetLabel() string {
	name := m.presetName()
	for _, p := range rename.Presets {
		if p.Name == name && p.Flags != "" && p.Format == m.format {
			return name + " (" + p.Flags + ")"
		}
	}
	return name
}

// save writes the format to the configuration file as its format.
func (m *tuiModel) save() {
	if m.tplErr != "" {
		m.notice = m.st.Failure.Render("not saved: " + m.tplErr)
		return
	}
	if err := config.SaveFormat(m.cfgPath, m.format); err != nil {
		m.notice = m.st.Failure.Render("not saved: " + err.Error())
		return
	}
	m.notice = m.st.Success.Render(iconSuccess) + m.st.Dim.Render(" saved as the format in "+abbrevHome(m.cfgPath, os.Getenv("HOME")))
}

// ================= view =================

// listHeight is how many lines the list or the tree gets: as many as it
// has, within the screen less the header, the format's box, the parts,
// the counts, a line and the keys.
func (m tuiModel) listHeight() int {
	return max(0, m.height-fixedLines)
}

// fixedLines counts the lines besides the list: the header, the format's
// box, the parts, the counts, a line and the keys.
const fixedLines = 8

// minHeight is the lowest terminal the screen is drawn on: below, the
// keys and the question would be cut off, and nothing can be confirmed.
const minHeight = fixedLines + 3

// tooLow reports whether the terminal is lower than minHeight.
func (m tuiModel) tooLow() bool {
	return m.height > 0 && m.height < minHeight
}

// highlight draws line, over width, as the cursor's.
func (m tuiModel) highlight(line string, width int) string {
	return m.st.CursorLine(padRight(line, width), m.uiState() == stateEdit)
}

// View draws the screen on the alternate screen, which is left when the
// program ends: the run is drawn after it, in the shell's scrollback.
func (m tuiModel) View() tea.View {
	// The last frame stays on the alternate screen too: leaving it for the
	// main screen there would draw an empty frame over the lines above.
	v := tea.NewView("")
	v.AltScreen = true
	if m.result != tuiRunning {
		return v
	}
	v.SetContent(strings.Join(m.lines(time.Now()), "\n"))
	if m.mouseOn() {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

func (m tuiModel) lines(now time.Time) []string {
	w := m.width
	if m.uiState() == stateTooLow {
		return []string{fitLine(" "+m.help(), w)}
	}
	editing := m.uiState() == stateEdit
	var lines []string
	// the header
	left := " " + m.st.Strong.Render("naminator") + "  " + fmt.Sprintf("%d %s in %s", len(m.rows), plural(len(m.rows), "photo", "photos"), m.where())
	right := m.st.Dim.Render("view: " + m.view.String() + " ")
	if m.nRead < len(m.rows) {
		right = m.spinner.View() + m.st.Dim.Render(fmt.Sprintf(" reading EXIF %d/%d ", m.nRead, len(m.rows)))
	}
	lines = append(lines, spread(left, right, w))

	// the format, after the directory it starts from
	border := m.st.Dim
	if editing {
		border = m.st.Part
	}
	inner := w - 2
	title := m.st.Strong.Render("Format")
	if editing {
		title = m.st.PartStrong.Render("Format")
	}
	lines = append(lines, border.Render("╭─ ")+title+border.Render(" "+strings.Repeat("─", max(0, inner-9))+"╮"))
	boxRight := m.st.Dim.Render(m.boxHint()+m.boxTail()) + " "
	boxLeft := " " + m.st.Path.Render(m.base()) + m.formatView()
	// a long format is cut rather than the hint on the right
	if room := inner - ansi.StringWidth(boxRight) - 1; ansi.StringWidth(boxLeft) > room {
		boxLeft = ansi.Truncate(boxLeft, max(room, 1), iconEllipsis)
	}
	body := spread(boxLeft, boxRight, inner)
	lines = append(lines, border.Render("│")+padRight(body, inner)+border.Render("│"))
	lines = append(lines, border.Render("╰"+strings.Repeat("─", max(0, inner))+"╯"))

	// the parts of the selected photo, or what is wrong with the format
	if m.tplErr != "" {
		lines = append(lines, fitLine("  "+m.st.Failure.Render(iconError+" "+m.tplErr), w))
	} else {
		lines = append(lines, fitLine(m.partsLine(), w))
	}

	// the list or the tree, the cursor's line on a lighter background
	items := m.items()
	win := m.window()
	if win.up {
		lines = append(lines, fitLine("     "+m.st.Dim.Render(fmt.Sprintf("↑ %d more  (%s)", win.top, m.keys.Hint(keymap.List, keymap.PageUp))), w))
	}
	for j := win.top; j < win.top+win.rows; j++ {
		if j == m.sel && !m.st.Plain {
			lines = append(lines, m.highlight(fitLine(items[j].draw(false), w), w))
		} else {
			lines = append(lines, fitLine(items[j].draw(j == m.sel), w))
		}
	}
	if win.down {
		lines = append(lines, fitLine("     "+m.st.Dim.Render(fmt.Sprintf("↓ %d more  (%s)", len(items)-win.top-win.rows, m.keys.Hint(keymap.List, keymap.PageDown))), w))
	}
	// the counts and the keys stay at the bottom of the screen
	for range m.listHeight() - win.rows - btoi(win.up) - btoi(win.down) {
		lines = append(lines, "")
	}

	// what the photos would get, then the keys
	lines = append(lines, fitLine(m.countLine(), w))
	lines = append(lines, fitLine("  "+m.notice, w))
	lines = append(lines, fitLine("  "+m.help(), w))
	return lines
}

// where names what the photos were given as: the argument, or how many.
func (m tuiModel) where() string {
	home := os.Getenv("HOME")
	names := make([]string, len(m.args))
	for i, a := range m.args {
		names[i] = abbrevHome(filepath.Clean(a), home)
	}
	return strings.Join(names, ", ")
}

// base is the directory the format starts from, for the selected photo.
func (m tuiModel) base() string {
	if m.namer == nil || len(m.rows) == 0 {
		return ""
	}
	r := m.selected()
	if r == nil {
		r = m.rows[0]
	}
	p := r.photo
	if !r.read {
		p = photo.Photo{Path: r.key, Dir: filepath.Dir(r.key)}
	}
	return m.shortDir(rename.AbsPath(m.namer.Output(p))) + "/"
}

// shortDir shows dir from the home directory, and only its last two
// directories when it is long, so that it does not push the rest away.
func (m tuiModel) shortDir(dir string) string {
	dir = abbrevHome(dir, os.Getenv("HOME"))
	if ansi.StringWidth(dir) > m.width/3 {
		if parts := strings.Split(dir, string(filepath.Separator)); len(parts) > 2 {
			dir = iconEllipsis + string(filepath.Separator) + filepath.Join(parts[len(parts)-2:]...)
		}
	}
	return dir
}

// formatView draws the format: placeholders in magenta, unknown or
// unclosed ones in red, and the cursor in insert mode.
func (m tuiModel) formatView() string {
	runes := []rune(m.format)
	editing := m.uiState() == stateEdit
	styles := make([]func(...string) string, len(runes))
	for i := 0; i < len(runes); {
		if runes[i] != '{' {
			styles[i] = m.st.Strong.Render
			i++
			continue
		}
		end := i + 1
		for end < len(runes) && runes[end] != '}' {
			end++
		}
		style := m.st.Failure.Render
		if end < len(runes) && slices.Contains(rename.Placeholders, string(runes[i+1:end])) {
			style = m.st.Part.Render
		}
		for j := i; j <= end && j < len(runes); j++ {
			styles[j] = style
		}
		i = end + 1
	}
	var b strings.Builder
	for i, r := range runes {
		if editing && i == m.cursor {
			b.WriteString(reverse(string(r)))
			continue
		}
		b.WriteString(styles[i](string(r)))
	}
	if editing && m.cursor >= len(runes) {
		b.WriteString(reverse(" "))
	}
	return b.String()
}

// reverse draws s as the cursor, in reverse video.
func reverse(s string) string {
	return "\x1b[7m" + s + "\x1b[27m"
}

// partsLine shows the values of the parts for the selected photo.
func (m tuiModel) partsLine() string {
	if len(m.rows) == 0 {
		return ""
	}
	r := m.selected()
	switch {
	case r == nil:
		return "  " + m.st.Dim.Render(fmt.Sprintf("a directory: %s folds it, %s opens it", m.keys.Hint(keymap.List, keymap.Fold), m.keys.Hint(keymap.List, keymap.Unfold)))
	case !r.read:
		return "  " + m.st.Dim.Render("reading EXIF of the selected photo…")
	case r.skipped():
		return "  " + m.st.Dim.Render(fmt.Sprintf("no shooting time in its EXIF: it is skipped (%s uses the file's time)", m.keys.Hint(keymap.List, keymap.FileTime)))
	case m.namer == nil:
		return ""
	}
	dir := ""
	if rel, err := filepath.Rel(m.namer.Input(r.photo), r.photo.Dir); err == nil && rel != "." {
		dir = rel
	}
	var b strings.Builder
	b.WriteString(" ")
	for _, k := range []string{"dir", "date", "time", "ext", "camera", "orig"} {
		v := rename.Value(r.photo, k, dir)
		if v == "" {
			v = "(none)"
		}
		b.WriteString(" " + m.st.Part.Render("{"+k+"}") + " " + m.st.Dim.Render(v) + " ")
	}
	return b.String()
}

// row draws one photo: where it is and where it would go.
func (m tuiModel) row(r *tuiRow, selected bool, fromW int) string {
	lead := "  "
	if selected {
		lead = m.st.Active.Render(">") + " "
	}
	from := padName(m.shown(r, r.key), fromW)
	switch {
	case !r.read:
		return lead + m.st.Dim.Render(iconQueued) + " " + m.st.Dim.Render(from) + "  " + m.st.Dim.Render("reading EXIF…")
	case r.skipped():
		return lead + m.st.Failure.Render(iconError) + " " + from + "  " + m.st.Dim.Render("no shooting time; skipped")
	case m.namer == nil:
		return lead + m.st.Dim.Render(iconQueued) + " " + from + "  " + m.st.Dim.Render("→ ?")
	case r.same:
		return lead + m.st.Success.Render(iconSuccess) + " " + m.st.Dim.Render(from) + "  " + m.st.Dim.Render("already named")
	}
	to := m.shown(r, r.target)
	if r.suffix != "" {
		base := strings.TrimSuffix(to, filepath.Ext(to))
		to = strings.TrimSuffix(base, r.suffix) + m.st.Warn.Render(r.suffix) + filepath.Ext(to)
	}
	note := ""
	if scs := m.sidecars[r.photo.Path]; len(scs) > 0 {
		var exts []string
		for _, sc := range scs {
			exts = append(exts, strings.ToLower(strings.TrimPrefix(filepath.Ext(sc), ".")))
		}
		note = m.st.Dim.Render("  + " + strings.Join(exts, " "))
	}
	mark := m.st.Warn.Render(iconChange)
	if r.fileTime {
		mark = m.st.Warn.Render(iconFileTime)
		note += m.st.Dim.Render("  (file time)")
	}
	return lead + mark + " " + from + "  " + m.st.Dim.Render("→") + " " + to + note
}

// countLine counts what the photos would get.
func (m tuiModel) countLine() string {
	if m.tplErr != "" {
		return "  " + m.st.Failure.Render("fix the format to see the new paths")
	}
	var rename, same, numbered, failed, reading int
	for _, r := range m.rows {
		switch {
		case !r.read:
			reading++
		case r.skipped():
			failed++
		case r.same:
			same++
		default:
			rename++
			if r.suffix != "" {
				numbered++
			}
		}
	}
	var parts []string
	if reading > 0 {
		parts = append(parts, m.st.Dim.Render(fmt.Sprintf("%d reading", reading)))
	}
	parts = append(parts, fmt.Sprintf("%d to rename", rename))
	if same > 0 {
		parts = append(parts, m.st.Dim.Render(fmt.Sprintf("%d already named", same)))
	}
	if numbered > 0 {
		parts = append(parts, m.st.Warn.Render(fmt.Sprintf("%d numbered", numbered)))
	}
	if failed > 0 {
		parts = append(parts, m.st.Failure.Render(fmt.Sprintf("%d without a shooting time", failed)))
	}
	if n := m.countFileTime(); n > 0 {
		parts = append(parts, m.st.Warn.Render(fmt.Sprintf("%d by file time", n)))
	}
	if n := len(m.removed); n > 0 {
		parts = append(parts, m.st.Dim.Render(fmt.Sprintf("%d %s removed", n, plural(n, "directory", "directories"))))
	}
	return "  " + joinDim(m.st, parts)
}

// countMoves counts the photos that get another path.
func (m tuiModel) countMoves() int {
	n := 0
	for _, r := range m.rows {
		if r.usable() && !r.same && r.target != "" {
			n++
		}
	}
	return n
}

// countFileTime counts the photos given the file's time.
func (m tuiModel) countFileTime() int {
	n := 0
	for _, r := range m.rows {
		if r.fileTime {
			n++
		}
	}
	return n
}

// help lists the keys of the mode, or asks to confirm.
func (m tuiModel) help() string {
	if m.uiState() == stateTooLow {
		return fmt.Sprintf("naminator needs a terminal of %d lines or more (%s quits)", minHeight, m.keys.Hint(keymap.TooLow, keymap.Quit))
	}
	if m.uiState() == stateConfirm {
		return m.confirmHelp()
	}
	return m.keyLine(m.hints(), m.width-2)
}

// confirmHelp asks whether to rename.
func (m tuiModel) confirmHelp() string {
	n := m.countMoves()
	what := fmt.Sprintf("Rename %d %s?", n, plural(n, "photo", "photos"))
	if m.dry {
		what = fmt.Sprintf("Try renaming %d %s (dry run)?", n, plural(n, "photo", "photos"))
	}
	return m.st.Warn.Render(what) + " " + m.st.Strong.Render(m.keys.Hint(keymap.Confirm, keymap.Yes)) + m.st.Dim.Render("/") + m.st.Strong.Render("N")
}

// hint is a key of the key line and what it does.
type hint struct {
	key, what string
	// keep is for those that stay on a narrow terminal, and off for one
	// that does nothing now
	keep, off bool
}

// hints are the keys that work in the list, the tree and while the format
// is edited.
func (m tuiModel) hints() []hint {
	k := func(scope keymap.Scope, what string, actions ...keymap.Action) hint {
		return hint{key: m.keys.Hint(scope, actions...), what: what}
	}
	switch m.uiState() {
	case stateEdit:
		return []hint{
			{key: m.keys.HintAll(keymap.Editing, keymap.Done), what: "done", keep: true},
			k(keymap.Screen, "preset", keymap.NextPreset),
			k(keymap.Editing, "cursor", keymap.CursorLeft, keymap.CursorRight),
			k(keymap.Screen, "move", keymap.Up, keymap.Down),
			k(keymap.Screen, "tree", keymap.ToggleView),
			k(keymap.Screen, "save", keymap.SaveFormat),
		}
	case stateList, stateTree:
		hints := []hint{
			k(keymap.List, "edit format", keymap.Edit),
			k(keymap.Screen, "preset", keymap.NextPreset),
			k(keymap.List, "move", keymap.Down, keymap.Up),
		}
		if m.uiState() == stateTree {
			hints = append(hints, k(keymap.List, "fold", keymap.Fold, keymap.Unfold), k(keymap.List, "list", keymap.ToggleView))
		} else {
			hints = append(hints, k(keymap.List, "tree", keymap.ToggleView))
		}
		if r := m.selected(); r != nil && r.read && r.err != "" {
			hints = append(hints, k(keymap.List, "file time", keymap.FileTime))
		}
		rename := k(keymap.List, "rename…", keymap.Rename)
		rename.keep = true
		if m.dry {
			rename.what = "dry run…"
		}
		if ok, why := m.ready(); !ok {
			rename.what, rename.off = "rename ("+why+")", true
		}
		quit := k(keymap.List, "quit", keymap.Quit)
		quit.keep = true
		return append(hints, rename, k(keymap.Screen, "save", keymap.SaveFormat), quit)
	}
	return nil
}

// keyLine draws hints in width, leaving out from the end those that do not
// fit and need not stay.
func (m tuiModel) keyLine(hints []hint, width int) string {
	draw := func() string {
		keys := make([]string, len(hints))
		for i, h := range hints {
			keys[i] = h.key + m.st.Dim.Render(" "+h.what)
			if h.off {
				keys[i] = m.st.Dim.Render(h.key + " " + h.what)
			}
		}
		return strings.Join(keys, m.st.Dim.Render(" • "))
	}
	for ansi.StringWidth(draw()) > width {
		i := len(hints) - 1
		for i >= 0 && hints[i].keep {
			i--
		}
		if i < 0 {
			break
		}
		hints = slices.Delete(hints, i, i+1)
	}
	return draw()
}

// spread puts left and right on one line of width cells.
func spread(left, right string, width int) string {
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		return fitLine(left, width)
	}
	return left + strings.Repeat(" ", gap) + right
}

// page moves the cursor and the lines shown by a page of the list, up
// (-1) or down (1), as a pager does.
func (m *tuiModel) page(dir int) {
	n := max(1, m.window().rows) * dir
	m.sel = max(0, min(len(m.items())-1, m.sel+n))
	m.top = max(0, m.top+n)
}

// listWindow is what the list shows: rows lines from top, and a line
// saying how many more there are above (up) and below (down).
type listWindow struct {
	top, rows int
	up, down  bool
}

// window lays out the list's lines: the lines from m.top, moved only to
// keep the cursor in, and, when they do not all fit, a line for those
// above and one for those below in place of a line of the list. A list
// of fewer than three lines has no room for them.
func (m tuiModel) window() listWindow {
	n, total := m.listHeight(), len(m.items())
	sel := max(0, min(m.sel, total-1))
	if total <= n || n < 3 {
		top := min(max(0, min(m.top, total-n)), sel)
		if sel >= top+n {
			top = sel - n + 1
		}
		return listWindow{top: max(0, top), rows: min(n, total)}
	}
	// the lines from top, with a line for those above when top is not the
	// first and one for those below when the last is not shown; moving top
	// for the cursor can add the line above, so lay it out again
	top := max(0, min(m.top, total-(n-1)))
	var w listWindow
	for range 4 {
		w = listWindow{top: top, rows: n, up: top > 0}
		w.rows -= btoi(w.up)
		w.down = top+w.rows < total
		w.rows -= btoi(w.down)
		switch {
		case sel < top:
			top = sel
		case sel >= top+w.rows:
			top = sel - w.rows + 1
		default:
			return w
		}
	}
	return w
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// shown writes a path of a row from the parent of the argument the photo
// was found under, as the arguments are usually given ("card/DSC00001.JPG"
// for ~/Desktop/card), however they were given. A path outside it, such
// as under -d, is written from the home directory.
func (m tuiModel) shown(r *tuiRow, path string) string {
	base := rename.AbsPath(rename.ArgParent(m.args, r.key))
	if rel, err := filepath.Rel(base, rename.AbsPath(path)); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return rel
	}
	return abbrevHome(rename.AbsPath(path), os.Getenv("HOME"))
}

// padRight pads s with spaces to width cells, or cuts it.
func padRight(s string, width int) string {
	if n := ansi.StringWidth(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return ansi.Truncate(s, width, iconEllipsis)
}
