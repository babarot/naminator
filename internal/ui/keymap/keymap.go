// Package keymap binds keys to actions by name. The screens never compare a
// key: they ask which action it is where they are, and draw the keys they
// tell of from here.
//
// Keys are written as bubbletea reports key presses: "a", "A", "enter",
// "ctrl+s", "shift+tab".
package keymap

import (
	"slices"
	"strings"
)

// Action is something a key does.
type Action string

// The actions.
const (
	// None is the action of a key that is not bound.
	None Action = ""

	Interrupt  Action = "interrupt"
	Quit       Action = "quit"
	Up         Action = "up"
	Down       Action = "down"
	Top        Action = "top"
	Bottom     Action = "bottom"
	PageUp     Action = "page_up"
	PageDown   Action = "page_down"
	Fold       Action = "fold"
	Unfold     Action = "unfold"
	ToggleView Action = "toggle_view"
	NextPreset Action = "next_preset"
	PrevPreset Action = "prev_preset"
	SaveFormat Action = "save_format"
	FileTime   Action = "file_time"
	Rename     Action = "rename"
	Yes        Action = "yes"

	Edit        Action = "edit"
	Append      Action = "append"
	Done        Action = "done"
	CursorLeft  Action = "cursor_left"
	CursorRight Action = "cursor_right"
	CursorStart Action = "cursor_start"
	CursorEnd   Action = "cursor_end"
	DeleteBack  Action = "delete_back"
)

// Scope is where a key is read. What the screen is in reads one or more of
// them, and a key may be bound to two actions only in scopes that are never
// read together.
type Scope int

// The scopes.
const (
	Anywhere Scope = iota
	// Screen is the list, the tree and editing the format.
	Screen
	// List is the list and the tree.
	List
	// Editing is editing the format.
	Editing
	// Confirm is asking y/N.
	Confirm
	// TooLow is a terminal too low to draw the screen.
	TooLow
)

// Def binds keys to an action in a scope. The first key is the one hints
// show.
type Def struct {
	Action Action
	Scope  Scope
	Keys   []string
}

// Defaults are the keys of every action.
var Defaults = []Def{
	{Interrupt, Anywhere, []string{"ctrl+c"}},

	{Up, Screen, []string{"up"}},
	{Down, Screen, []string{"down"}},
	{PageUp, Screen, []string{"pgup"}},
	{PageDown, Screen, []string{"pgdown"}},
	{NextPreset, Screen, []string{"tab"}},
	{PrevPreset, Screen, []string{"shift+tab"}},
	{ToggleView, Screen, []string{"ctrl+t"}},
	{SaveFormat, Screen, []string{"ctrl+s"}},

	{Edit, List, []string{"i"}},
	{Append, List, []string{"a"}},
	{Down, List, []string{"j"}},
	{Up, List, []string{"k"}},
	{PageUp, List, []string{"ctrl+u"}},
	{PageDown, List, []string{"ctrl+d"}},
	{Top, List, []string{"g"}},
	{Bottom, List, []string{"G"}},
	{Fold, List, []string{"h", "left"}},
	{Unfold, List, []string{"l", "right"}},
	{ToggleView, List, []string{"v"}},
	{FileTime, List, []string{"m"}},
	{Rename, List, []string{"enter"}},
	{Quit, List, []string{"q"}},

	{Done, Editing, []string{"esc", "enter"}},
	{CursorLeft, Editing, []string{"left"}},
	{CursorRight, Editing, []string{"right"}},
	{CursorStart, Editing, []string{"home", "ctrl+a"}},
	{CursorEnd, Editing, []string{"end", "ctrl+e"}},
	{DeleteBack, Editing, []string{"backspace"}},

	{Yes, Confirm, []string{"y"}},

	{Quit, TooLow, []string{"q"}},
}

// Map is the keys of the actions.
type Map struct {
	defs []Def
}

// Default is the map of Defaults.
func Default() Map { return Map{defs: Defaults} }

// Action returns the action of key in scopes, None when it is not bound in
// any of them.
func (m Map) Action(key string, scopes ...Scope) Action {
	for _, d := range m.defs {
		if slices.Contains(scopes, d.Scope) && slices.Contains(d.Keys, key) {
			return d.Action
		}
	}
	return None
}

// Keys returns the keys of action in scope.
func (m Map) Keys(scope Scope, action Action) []string {
	for _, d := range m.defs {
		if d.Scope == scope && d.Action == action {
			return d.Keys
		}
	}
	return nil
}

// Hint is how a hint shows actions in scope: the first key of each, as
// "j/k".
func (m Map) Hint(scope Scope, actions ...Action) string {
	var out []string
	for _, a := range actions {
		if keys := m.Keys(scope, a); len(keys) > 0 {
			out = append(out, Glyph(keys[0]))
		}
	}
	return strings.Join(out, "/")
}

// HintAll is how a hint shows every key of action in scope, as "esc/enter".
func (m Map) HintAll(scope Scope, action Action) string {
	keys := slices.Clone(m.Keys(scope, action))
	for i, k := range keys {
		keys[i] = Glyph(k)
	}
	return strings.Join(keys, "/")
}

// Glyph is how a key is shown: an arrow for an arrow key.
func Glyph(key string) string {
	switch key {
	case "up":
		return "↑"
	case "down":
		return "↓"
	case "left":
		return "←"
	case "right":
		return "→"
	}
	return key
}
