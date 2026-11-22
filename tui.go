package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
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

// tuiResult is how the screen ended.
type tuiResult int

const (
	tuiRunning   tuiResult = iota
	tuiConfirmed           // rename with the format
	tuiQuit                // q: nothing renamed
	tuiCanceled            // Ctrl+C or a signal: nothing renamed
)

// tuiDecision is what the screen decided: the result, and the format to
// rename with when confirmed.
type tuiDecision struct {
	result tuiResult
	format string
	// failed are the photos whose EXIF data could not be read, to report,
	// and fileTime those given the file's time instead, to rename too
	failed   []failure
	fileTime []Photo
	// targets are the paths previewed, by photo path: the run gives them,
	// so that the photos go where they were shown to go
	targets map[string]string
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
	photo Photo
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
	fs      FileSystem
	cfgPath string
	presets []preset
	cancel  func()
	// runBase and runInto name where the photos are, for the run's display
	runBase string
	runInto bool
	// decided tells the work what to do once the screen is done
	decided chan<- tuiDecision
	// run is the usual display, which takes over once confirmed
	run *model

	rows   []*tuiRow
	byKey  map[string]*tuiRow
	nRead  int
	format string
	cursor int // in runes of format
	namer  *namer
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

	// dark tells the terminal's background, for the color of the cursor's
	// line
	dark bool
	// cursorY is the row the prompt left the cursor on, -1 until the
	// terminal tells; settled is set once it has, or the wait is over:
	// nothing is drawn before, so that the answer is where the prompt is
	cursorY int
	settled bool

	insert  bool
	confirm bool
	sel     int
	top     int
	notice  string
	width   int
	height  int
	spinner spinner.Model
	result  tuiResult
}

func newTUIModel(args, images []string, format, destDir string, dry, clean bool, presets []preset, fsys FileSystem, cfgPath string, cancel func()) tuiModel {
	width, height := 100, 30
	if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		width, height = w, h
	}
	m := tuiModel{
		args: args, destDir: destDir, dry: dry, clean: clean, fs: fsys, cfgPath: cfgPath, presets: presets, cancel: cancel,
		byKey:    map[string]*tuiRow{},
		folded:   map[string]bool{},
		expanded: map[string]bool{},
		cursorY:  -1,
		width:    width,
		height:   height,
		spinner:  spinner.New(spinner.WithSpinner(spinner.Jump), spinner.WithStyle(Cyan)),
	}
	for _, img := range images {
		r := &tuiRow{key: img}
		m.rows = append(m.rows, r)
		m.byKey[img] = r
	}
	m.setFormat(format)
	return m
}

func (m tuiModel) Init() tea.Cmd {
	// The screen reaches from the prompt down, as gomi's does: ask where
	// the cursor is before drawing; a terminal that does not tell gets
	// half its rows after a while
	return tea.Batch(m.spinner.Tick, tea.RequestCursorPosition, tea.Tick(cursorWait, func(time.Time) tea.Msg { return cursorWaitedMsg{} }))
}

// cursorWait is how long the screen waits for the terminal to tell where
// the cursor is before drawing without it.
var cursorWait = 250 * time.Millisecond

// cursorWaitedMsg says the wait for the cursor position is over.
type cursorWaitedMsg struct{}

// setFormat sets the format, puts the cursor at its end, and plans again.
func (m *tuiModel) setFormat(f string) {
	m.format = f
	m.cursor = len([]rune(f))
	m.plan()
}

// plan gives each read photo its path with the format: in the order the
// photos are taken, through a reserver, as the run will.
func (m *tuiModel) plan() {
	tpl, err := parseTemplate(m.format)
	if err != nil {
		m.tplErr, m.namer = err.Error(), nil
		return
	}
	m.tplErr = ""
	m.removed = nil
	m.namer = newNamer(tpl, m.args, m.destDir)
	var photos []Photo
	for _, r := range m.rows {
		if r.usable() {
			photos = append(photos, r.photo)
		}
	}
	sortByTime(photos)
	reserver := newPathReserver()
	for _, p := range photos {
		r := m.byKey[m.keyOf(p)]
		want := m.namer.target(p)
		r.target = reserver.reserve(m.fs, p.Path, want)
		r.same = r.target == p.Path
		r.suffix = strings.TrimPrefix(strings.TrimSuffix(r.target, filepath.Ext(r.target)), strings.TrimSuffix(want, filepath.Ext(want)))
	}
	m.removed = nil
	if m.clean && m.nRead == len(m.rows) {
		m.removed = m.predictRemoved()
	}
}

// keyOf returns the row key of a photo read: the path it was read from.
func (m tuiModel) keyOf(p Photo) string {
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
func (m tuiModel) photos() []Photo {
	var out []Photo
	for _, r := range m.rows {
		if r.usable() {
			out = append(out, r.photo)
		}
	}
	return out
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	if tm, ok := next.(tuiModel); ok && tm.run == nil {
		// The lines can be fewer after a photo is read or the format
		// changes: keep the cursor on one of them
		tm.sel = max(0, min(tm.sel, len(tm.items())-1))
		// keep the lines shown, moving them only to keep the cursor in
		n := tm.listHeight()
		tm.top = min(max(0, min(tm.top, len(tm.items())-n)), tm.sel)
		if tm.sel >= tm.top+n {
			tm.top = tm.sel - n + 1
		}
		return tm, cmd
	}
	return next, cmd
}

func (m tuiModel) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.run != nil {
		next, cmd := m.run.Update(msg)
		run, _ := next.(model)
		m.run = &run
		return m, cmd
	}
	// Confirmed, the screen waits a frame before the display takes over:
	// nothing may change the format confirmed, only Ctrl+C give up.
	if m.result == tuiConfirmed {
		switch msg := msg.(type) {
		case handoverMsg:
			return m.handover()
		case interruptMsg:
			return m.end(tuiCanceled)
		case tea.KeyPressMsg:
			if msg.String() == "ctrl+c" {
				return m.end(tuiCanceled)
			}
		}
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.CursorPositionMsg:
		// it may come after the wait; asked before anything was drawn, it
		// still tells where the prompt left the cursor
		m.cursorY, m.settled = msg.Y, true
		return m, nil

	case cursorWaitedMsg:
		m.settled = true
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
			m.sidecars, _ = findSidecars(m.fs, m.photos())
		}
		m.plan()
		return m, nil

	case interruptMsg:
		return m.end(tuiCanceled)

	case tea.KeyPressMsg:
		return m.key(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

// end closes the screen. When confirmed, the usual display takes over
// from the rename phase, and the program quits once the run is done;
// otherwise the EXIF reading stops and the program quits.
func (m tuiModel) end(r tuiResult) (tea.Model, tea.Cmd) {
	m.result = r
	if r != tuiConfirmed {
		m.decide()
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	}
	// Let a frame without the screen be drawn first: the display drawn
	// over the taller screen at once would leave lines of it behind.
	return m, tea.Tick(handoverDelay, func(time.Time) tea.Msg { return handoverMsg{} })
}

// handoverMsg hands the program over to the usual display, after a frame
// without the screen.
type handoverMsg struct{}

// handoverDelay is how long the empty frame is given: more than a frame at
// the renderer's 60 FPS.
const handoverDelay = 50 * time.Millisecond

// decide tells the work what the screen decided. When confirmed, it is
// told once the display has taken over, so that the renames it reports
// reach the display.
func (m tuiModel) decide() {
	if m.decided == nil {
		return
	}
	d := tuiDecision{result: m.result, format: m.format, targets: map[string]string{}}
	for _, row := range m.rows {
		if row.usable() && row.target != "" {
			d.targets[row.photo.Path] = row.target
		}
		switch {
		case row.skipped():
			d.failed = append(d.failed, failure{name: filepath.Base(row.key), err: row.err})
		case row.fileTime:
			d.fileTime = append(d.fileTime, row.photo)
		}
	}
	m.decided <- d
}

// handover makes the usual display, from the rename phase, take over.
func (m tuiModel) handover() (tea.Model, tea.Cmd) {
	photos := m.photos()
	sortByTime(photos)
	run := newModel(phaseStartMsg{kind: phaseRename, keys: pathsOf(photos), base: m.runBase, into: m.runInto, dry: m.dry}, m.cancel)
	run.width, run.height = m.width, m.height
	if run.phase.wantCompact(run.height) {
		run.phase.compact = true
		run.queue = append(run.queue, run.phase.title(), "")
	}
	cmd := run.flush()
	m.run = &run
	m.decide()
	return m, tea.Batch(cmd, run.Init())
}

// ready reports whether the photos can be renamed, and why not.
func (m tuiModel) ready() (bool, string) {
	switch {
	case m.nRead < len(m.rows):
		return false, "after EXIF"
	case m.tplErr != "":
		return false, "fix the format"
	case m.countMoves() == 0:
		return false, "nothing to rename"
	}
	return true, ""
}

func (m tuiModel) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	m.notice = ""
	if k == "ctrl+c" {
		return m.end(tuiCanceled)
	}
	if m.confirm {
		m.confirm = false
		if k == "y" {
			return m.end(tuiConfirmed)
		}
		return m, nil
	}
	// in both modes
	switch k {
	case "tab", "shift+tab":
		m.cyclePreset(k == "tab")
		return m, nil
	case "up":
		m.sel = max(0, m.sel-1)
		return m, nil
	case "down":
		m.sel = min(len(m.items())-1, m.sel+1)
		return m, nil
	case "pgup":
		m.sel = max(0, m.sel-10)
		return m, nil
	case "pgdown":
		m.sel = min(len(m.items())-1, m.sel+10)
		return m, nil
	case "ctrl+t":
		m.toggleView()
		return m, nil
	case "ctrl+s":
		m.save()
		return m, nil
	}
	if m.insert {
		r := []rune(m.format)
		switch k {
		case "esc", "enter":
			m.insert = false
		case "left":
			m.cursor = max(0, m.cursor-1)
		case "right":
			m.cursor = min(len(r), m.cursor+1)
		case "home", "ctrl+a":
			m.cursor = 0
		case "end", "ctrl+e":
			m.cursor = len(r)
		case "backspace":
			if m.cursor > 0 {
				m.format = string(r[:m.cursor-1]) + string(r[m.cursor:])
				m.cursor--
				m.plan()
			}
		default:
			if t := msg.Text; t != "" && msg.Mod == 0 || msg.Mod == tea.ModShift && t != "" {
				m.format = string(r[:m.cursor]) + t + string(r[m.cursor:])
				m.cursor += len([]rune(t))
				m.plan()
			}
		}
		return m, nil
	}
	switch k {
	case "i":
		m.insert = true
	case "a":
		m.insert, m.cursor = true, len([]rune(m.format))
	case "j":
		m.sel = min(len(m.items())-1, m.sel+1)
	case "k":
		m.sel = max(0, m.sel-1)
	case "g":
		m.sel = 0
	case "G":
		m.sel = len(m.items()) - 1
	case "h", "left":
		m.fold(false)
	case "l", "right":
		m.fold(true)
	case "t":
		m.toggleView()
	case "m":
		m.toggleFileTime()
	case "enter":
		if ok, _ := m.ready(); ok {
			m.confirm = true
		}
	case "q":
		return m.end(tuiQuit)
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
			m.notice = Red.Render("no file time: " + err.Error())
			return
		}
		r.fileTime = true
		r.photo = Photo{
			Name:      filepath.Base(r.key),
			Path:      r.key,
			Dir:       filepath.Dir(r.key),
			Extension: strings.ToLower(strings.TrimPrefix(filepath.Ext(r.key), ".")),
			CreatedAt: fi.ModTime(),
		}
	}
	if m.nRead == len(m.rows) {
		m.sidecars, _ = findSidecars(m.fs, m.photos())
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
		fromW = max(fromW, ansi.StringWidth(r.key))
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
		if p.format == m.format {
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
	m.setFormat(m.presets[i].format)
}

// presetName names the format when it is a preset.
func (m tuiModel) presetName() string {
	for _, p := range m.presets {
		if p.format == m.format {
			return p.name
		}
	}
	return "custom"
}

// presetLabel is presetName with the flags the preset stands for, as
// "by date (-t)".
func (m tuiModel) presetLabel() string {
	name := m.presetName()
	for _, p := range presets {
		if p.name == name && p.flags != "" && p.format == m.format {
			return name + " (" + p.flags + ")"
		}
	}
	return name
}

// save writes the format to the configuration file as its format.
func (m *tuiModel) save() {
	if m.tplErr != "" {
		m.notice = Red.Render("not saved: " + m.tplErr)
		return
	}
	if err := saveFormat(m.cfgPath, m.format); err != nil {
		m.notice = Red.Render("not saved: " + err.Error())
		return
	}
	m.notice = Green.Render(iconSuccess) + Dim.Render(" saved as the format in "+abbrevHome(m.cfgPath, os.Getenv("HOME")))
}

// saveFormat sets format in the configuration file at path: it replaces
// the line of format, or adds one at the top, and keeps the rest, comments
// included.
func saveFormat(path, format string) error {
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	line := "format: " + strconv.Quote(format)
	lines := strings.Split(string(b), "\n")
	done := false
	for i, l := range lines {
		if strings.HasPrefix(l, "format:") {
			lines[i], done = line, true
			break
		}
	}
	if !done {
		if len(b) == 0 {
			lines = []string{line, ""}
		} else {
			lines = append([]string{line}, lines...)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// ================= view =================

// listHeight is how many lines the list or the tree gets: as many as it
// has, within the screen less the header, the format's box, the parts,
// the counts, a line and the keys.
func (m tuiModel) listHeight() int {
	return max(0, min(len(m.items()), m.maxHeight()-m.fixedLines()))
}

// tight reports whether the terminal is too low for every line: the parts
// and the notice are then left out.
func (m tuiModel) tight() bool {
	return m.height > 0 && m.height-1 < 11
}

// fixedLines counts the lines besides the list: the header, the format's
// box, the parts, the counts, a line and the keys; two fewer when tight.
func (m tuiModel) fixedLines() int {
	switch {
	case m.tight() && m.tplErr != "":
		return 7 // the error stays
	case m.tight():
		return 6
	}
	return 8
}

// resetSGR matches the resets of the styles, after which the background of
// the cursor's line is set again.
var resetSGR = regexp.MustCompile(`\x1b\[0?m`)

// highlight draws line, padded to width, on the background of the cursor's
// line: lighter than the terminal's, and lighter still in insert mode,
// where the keys go to the format.
func (m tuiModel) highlight(line string, width int) string {
	color := "40;52;87" // #283457
	switch {
	case m.dark && m.insert:
		color = "34;36;54" // #222436
	case !m.dark && m.insert:
		color = "238;241;251" // #eef1fb
	case !m.dark:
		color = "219;228;255" // #dbe4ff
	}
	bg := "\x1b[48;2;" + color + "m"
	line = resetSGR.ReplaceAllStringFunc(line, func(r string) string { return r + bg })
	return bg + padRight(line, width) + "\x1b[49m"
}

// viewHeight is how many lines the screen takes: what it has to show.
func (m tuiModel) viewHeight() int {
	return m.listHeight() + m.fixedLines()
}

// maxHeight is the most lines the screen takes, as gomi's "auto" height:
// half the terminal, or the rows from the prompt down when there are more,
// less a line for the cursor, and at least a few rows of the list.
func (m tuiModel) maxHeight() int {
	n := m.height / 2
	if m.cursorY >= 0 {
		n = max(n, m.height-m.cursorY)
	}
	n = max(11, min(n, m.height)-1)
	if m.height > 0 {
		// never taller than the terminal, less the cursor's line
		n = min(n, m.height-1)
	}
	return n
}

func (m tuiModel) View() tea.View {
	if m.run != nil {
		return m.run.View()
	}
	if m.result != tuiRunning || !m.settled {
		return tea.NewView("")
	}
	return tea.NewView(strings.Join(m.lines(time.Now()), "\n"))
}

func (m tuiModel) lines(now time.Time) []string {
	w := m.width
	var lines []string
	// the header
	left := " " + Bold.Render("naminator") + "  " + fmt.Sprintf("%d %s in %s", len(m.rows), plural(len(m.rows), "photo", "photos"), m.where())
	right := Dim.Render("view: " + m.view.String() + " ")
	if m.nRead < len(m.rows) {
		right = m.spinner.View() + Dim.Render(fmt.Sprintf(" reading EXIF %d/%d ", m.nRead, len(m.rows)))
	}
	lines = append(lines, spread(left, right, w))

	// the format, after the directory it starts from
	border := Dim
	if m.insert {
		border = Magenta
	}
	inner := w - 2
	title := Bold.Render("Format")
	if m.insert {
		title = Magenta.Bold(true).Render("Format")
	}
	lines = append(lines, border.Render("╭─ ")+title+border.Render(" "+strings.Repeat("─", max(0, inner-9))+"╮"))
	hint := "i edit  "
	if m.insert {
		hint = "editing  "
	}
	boxRight := Dim.Render(hint+m.presetLabel()+"  tab ›") + " "
	boxLeft := " " + Blue.Render(m.base()) + m.formatView()
	// a long format is cut rather than the hint on the right
	if room := inner - ansi.StringWidth(boxRight) - 1; ansi.StringWidth(boxLeft) > room {
		boxLeft = ansi.Truncate(boxLeft, max(room, 1), iconEllipsis)
	}
	body := spread(boxLeft, boxRight, inner)
	lines = append(lines, border.Render("│")+padRight(body, inner)+border.Render("│"))
	lines = append(lines, border.Render("╰"+strings.Repeat("─", max(0, inner))+"╯"))

	// the parts of the selected photo, or what is wrong with the format;
	// the error stays on a low terminal, the parts do not
	if m.tight() && m.tplErr == "" {
		// left out
	} else if m.tplErr != "" {
		lines = append(lines, fitLine("  "+Red.Render(iconError+" "+m.tplErr), w))
	} else {
		lines = append(lines, fitLine(m.partsLine(), w))
	}

	// the list or the tree, the cursor's line on a lighter background
	items := m.items()
	n := m.listHeight()
	top := min(max(0, min(m.top, len(items)-n)), m.sel)
	if m.sel >= top+n {
		top = m.sel - n + 1
	}
	for i := range n {
		j := top + i
		switch {
		case j >= len(items):
			lines = append(lines, "")
		case j == m.sel && styled:
			lines = append(lines, m.highlight(fitLine(items[j].draw(false), w), w))
		default:
			lines = append(lines, fitLine(items[j].draw(j == m.sel), w))
		}
	}

	// what the photos would get, then the keys
	lines = append(lines, spread(m.countLine(), Dim.Render(fmt.Sprintf("%d/%d ", min(top+n, len(items)), len(items))), w))
	if !m.tight() {
		lines = append(lines, fitLine("  "+m.notice, w))
	}
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
		p = Photo{Path: r.key, Dir: filepath.Dir(r.key)}
	}
	return m.shortDir(absPath(m.namer.output(p))) + "/"
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
	styles := make([]func(...string) string, len(runes))
	for i := 0; i < len(runes); {
		if runes[i] != '{' {
			styles[i] = Bold.Render
			i++
			continue
		}
		end := i + 1
		for end < len(runes) && runes[end] != '}' {
			end++
		}
		style := Red.Render
		if end < len(runes) && slices.Contains(placeholders, string(runes[i+1:end])) {
			style = Magenta.Render
		}
		for j := i; j <= end && j < len(runes); j++ {
			styles[j] = style
		}
		i = end + 1
	}
	var b strings.Builder
	for i, r := range runes {
		if m.insert && i == m.cursor {
			b.WriteString(reverse(string(r)))
			continue
		}
		b.WriteString(styles[i](string(r)))
	}
	if m.insert && m.cursor >= len(runes) {
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
		return "  " + Dim.Render("a directory: h folds it, l opens it")
	case !r.read:
		return "  " + Dim.Render("reading EXIF of the selected photo…")
	case r.skipped():
		return "  " + Dim.Render("no shooting time in its EXIF: it is skipped (m uses the file's time)")
	case m.namer == nil:
		return ""
	}
	dir := ""
	if rel, err := filepath.Rel(m.namer.input(r.photo), r.photo.Dir); err == nil && rel != "." {
		dir = rel
	}
	var b strings.Builder
	b.WriteString(" ")
	for _, k := range []string{"dir", "date", "time", "ext", "camera", "orig"} {
		v := r.photo.value(k, dir)
		if v == "" {
			v = "(none)"
		}
		b.WriteString(" " + Magenta.Render("{"+k+"}") + " " + Dim.Render(v) + " ")
	}
	return b.String()
}

// row draws one photo: where it is and where it would go.
func (m tuiModel) row(r *tuiRow, selected bool, fromW int) string {
	lead := "  "
	if selected {
		lead = Cyan.Render(">") + " "
	}
	from := padName(r.key, fromW)
	switch {
	case !r.read:
		return lead + Dim.Render(iconQueued) + " " + Dim.Render(from) + "  " + Dim.Render("reading EXIF…")
	case r.skipped():
		return lead + Red.Render(iconError) + " " + from + "  " + Dim.Render("no shooting time; skipped")
	case m.namer == nil:
		return lead + Dim.Render(iconQueued) + " " + from + "  " + Dim.Render("→ ?")
	case r.same:
		return lead + Green.Render(iconSuccess) + " " + Dim.Render(from) + "  " + Dim.Render("already named")
	}
	to := r.target
	if r.suffix != "" {
		base := strings.TrimSuffix(to, filepath.Ext(to))
		to = strings.TrimSuffix(base, r.suffix) + Yellow.Render(r.suffix) + filepath.Ext(to)
	}
	note := ""
	if scs := m.sidecars[r.photo.Path]; len(scs) > 0 {
		var exts []string
		for _, sc := range scs {
			exts = append(exts, strings.ToLower(strings.TrimPrefix(filepath.Ext(sc), ".")))
		}
		note = Dim.Render("  + " + strings.Join(exts, " "))
	}
	mark := Yellow.Render(iconChange)
	if r.fileTime {
		mark = Yellow.Render(iconFileTime)
		note += Dim.Render("  (file time)")
	}
	return lead + mark + " " + from + "  " + Dim.Render("→") + " " + to + note
}

// countLine counts what the photos would get.
func (m tuiModel) countLine() string {
	if m.tplErr != "" {
		return "  " + Red.Render("fix the format to see the new paths")
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
		parts = append(parts, Dim.Render(fmt.Sprintf("%d reading", reading)))
	}
	parts = append(parts, fmt.Sprintf("%d to rename", rename))
	if same > 0 {
		parts = append(parts, Dim.Render(fmt.Sprintf("%d already named", same)))
	}
	if numbered > 0 {
		parts = append(parts, Yellow.Render(fmt.Sprintf("%d numbered", numbered)))
	}
	if failed > 0 {
		parts = append(parts, Red.Render(fmt.Sprintf("%d without a shooting time", failed)))
	}
	if n := m.countFileTime(); n > 0 {
		parts = append(parts, Yellow.Render(fmt.Sprintf("%d by file time", n)))
	}
	if n := len(m.removed); n > 0 {
		parts = append(parts, Dim.Render(fmt.Sprintf("%d %s removed", n, plural(n, "directory", "directories"))))
	}
	return "  " + joinDim(parts)
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
	if m.confirm {
		n := m.countMoves()
		what := fmt.Sprintf("Rename %d %s?", n, plural(n, "photo", "photos"))
		if m.dry {
			what = fmt.Sprintf("Try renaming %d %s (dry run)?", n, plural(n, "photo", "photos"))
		}
		return Yellow.Render(what) + " " + Bold.Render("y") + Dim.Render("/") + Bold.Render("N")
	}
	key := func(k, what string) string { return k + Dim.Render(" "+what) }
	if m.insert {
		return strings.Join([]string{key("esc/enter", "done"), key("tab", "preset"), key("←/→", "cursor"), key("↑/↓", "move"), key("ctrl+t", "tree"), key("ctrl+s", "save")}, Dim.Render(" • "))
	}
	enter := key("enter", "rename…")
	if m.dry {
		enter = key("enter", "dry run…")
	}
	if ok, why := m.ready(); !ok {
		enter = Dim.Render("enter rename (" + why + ")")
	}
	keys := []string{key("i", "edit format"), key("tab", "preset"), key("j/k", "move")}
	if m.view == viewTree {
		keys = append(keys, key("h/l", "fold"), key("t", "list"))
	} else {
		keys = append(keys, key("t", "tree"))
	}
	if r := m.selected(); r != nil && r.read && r.err != "" {
		keys = append(keys, key("m", "file time"))
	}
	keys = append(keys, enter, key("ctrl+s", "save"), key("q", "quit"))
	// on a narrow terminal, leave out ctrl+s, which is shown last otherwise
	if line := strings.Join(keys, Dim.Render(" • ")); ansi.StringWidth(line) > m.width-2 {
		keys = slices.DeleteFunc(keys, func(k string) bool { return strings.HasPrefix(k, "ctrl+s") })
	}
	return strings.Join(keys, Dim.Render(" • "))
}

// spread puts left and right on one line of width cells.
func spread(left, right string, width int) string {
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		return fitLine(left, width)
	}
	return left + strings.Repeat(" ", gap) + right
}

// padRight pads s with spaces to width cells, or cuts it.
func padRight(s string, width int) string {
	if n := ansi.StringWidth(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return ansi.Truncate(s, width, iconEllipsis)
}
