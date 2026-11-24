package ui

import "github.com/babarot/naminator/internal/ui/keymap"

// uiState is what the interactive screen is in, read from the fields that
// hold it, in the order keys are given out: what is asked or edited, then
// the list. Keys, the mouse, what is drawn and the key line go by it, and
// the tests run through every value, so that a state added here is one
// they cannot miss.
//
// Editing and confirming are over the list or the tree, so what is drawn
// in them still goes by the view.
type uiState int

const (
	stateList    uiState = iota // moving in the list
	stateTree                   // moving in the tree
	stateEdit                   // editing the format
	stateConfirm                // asking y/N
	stateTooLow                 // the terminal is too low to draw the screen
	numUIStates
)

var uiStateNames = [numUIStates]string{"list", "tree", "edit", "confirm", "too low"}

func (s uiState) String() string { return uiStateNames[s] }

func (m tuiModel) uiState() uiState {
	switch {
	case m.tooLow():
		return stateTooLow
	case m.confirm:
		return stateConfirm
	case m.insert:
		return stateEdit
	case m.view == viewTree:
		return stateTree
	}
	return stateList
}

// takesMouse reports whether the mouse does something: while nothing is
// asked, and the screen is drawn.
func (s uiState) takesMouse() bool { return s != stateConfirm && s != stateTooLow }

// scopes are where the keys of the state are read.
func (s uiState) scopes() []keymap.Scope {
	switch s {
	case stateList, stateTree:
		return []keymap.Scope{keymap.Anywhere, keymap.Screen, keymap.List}
	case stateEdit:
		return []keymap.Scope{keymap.Anywhere, keymap.Screen, keymap.Editing}
	case stateConfirm:
		return []keymap.Scope{keymap.Anywhere, keymap.Confirm}
	case stateTooLow:
		return []keymap.Scope{keymap.Anywhere, keymap.TooLow}
	}
	return []keymap.Scope{keymap.Anywhere}
}
