// Package statusline holds what the status line of the screen shows: what
// just happened, for a moment.
package statusline

import "time"

// Kind is what the line tells.
type Kind int

const (
	// None is an empty line.
	None Kind = iota
	// Info is a success or other information.
	Info
	// Refusal says why the key pressed cannot act here.
	Refusal
	// Failure says that something went wrong.
	Failure
)

// How long each kind stays: a failure longer, as it may need reading twice.
const (
	InfoFor    = 4 * time.Second
	RefusalFor = 4 * time.Second
	FailureFor = 10 * time.Second
)

// Line is the state of the status line. The zero Line is empty.
type Line struct {
	kind Kind
	text string
	end  time.Time
}

// Say tells of something that happened.
func (l *Line) Say(now time.Time, text string) {
	*l = Line{kind: Info, text: text, end: now.Add(InfoFor)}
}

// Refuse tells why the key pressed cannot act here. It goes at the next key.
func (l *Line) Refuse(now time.Time, text string) {
	*l = Line{kind: Refusal, text: text, end: now.Add(RefusalFor)}
}

// Fail tells of something that went wrong.
func (l *Line) Fail(now time.Time, text string) {
	*l = Line{kind: Failure, text: text, end: now.Add(FailureFor)}
}

// KeyPressed puts away a refusal, which answers the key before.
func (l *Line) KeyPressed() {
	if l.kind == Refusal {
		*l = Line{}
	}
}

// Shown is what the line shows at now.
func (l Line) Shown(now time.Time) (Kind, string) {
	if l.kind == None || !now.Before(l.end) {
		return None, ""
	}
	return l.kind, l.text
}

// Due is when what the line shows at now goes, zero when it shows nothing.
func (l Line) Due(now time.Time) time.Time {
	if k, _ := l.Shown(now); k == None {
		return time.Time{}
	}
	return l.end
}
