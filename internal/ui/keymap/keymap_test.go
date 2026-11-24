package keymap

import (
	"slices"
	"testing"
)

func TestAction(t *testing.T) {
	m := Default()
	for _, tt := range []struct {
		key    string
		scopes []Scope
		want   Action
	}{
		{"j", []Scope{Anywhere, Screen, List}, Down},
		{"down", []Scope{Anywhere, Screen, List}, Down},
		{"j", []Scope{Anywhere, Screen, Editing}, None},
		{"left", []Scope{Anywhere, Screen, List}, Fold},
		{"left", []Scope{Anywhere, Screen, Editing}, CursorLeft},
		{"ctrl+c", []Scope{Anywhere, Confirm}, Interrupt},
		{"q", []Scope{Anywhere, Confirm}, None},
		{"q", []Scope{Anywhere, TooLow}, Quit},
	} {
		if got := m.Action(tt.key, tt.scopes...); got != tt.want {
			t.Errorf("Action(%q, %v) = %q, want %q", tt.key, tt.scopes, got, tt.want)
		}
	}
}

func TestHint(t *testing.T) {
	m := Default()
	for got, want := range map[string]string{
		m.Hint(List, Down, Up):                     "j/k",
		m.Hint(Screen, Up, Down):                   "↑/↓",
		m.Hint(Editing, CursorLeft, CursorRight):   "←/→",
		m.HintAll(Editing, Done):                   "esc/enter",
		m.Hint(Confirm, Quit) + "|no key in scope": "|no key in scope",
	} {
		if got != want {
			t.Errorf("hint = %q, want %q", got, want)
		}
	}
}

// An action is bound once in a scope, and a key once, so that Keys and
// Action find the only one.
func TestDefaults(t *testing.T) {
	for i, d := range Defaults {
		if len(d.Keys) == 0 {
			t.Errorf("%s has no keys", d.Action)
		}
		for _, e := range Defaults[i+1:] {
			if d.Scope != e.Scope {
				continue
			}
			if d.Action == e.Action {
				t.Errorf("%s is bound twice in scope %d", d.Action, d.Scope)
			}
			for _, k := range d.Keys {
				if slices.Contains(e.Keys, k) {
					t.Errorf("%q is both %s and %s in scope %d", k, d.Action, e.Action, d.Scope)
				}
			}
		}
	}
}
