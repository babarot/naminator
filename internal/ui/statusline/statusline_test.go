package statusline

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 4, 15, 0, 0, 0, time.UTC)

func TestLine(t *testing.T) {
	var l Line
	if k, s := l.Shown(t0); k != None || s != "" || !l.Due(t0).IsZero() {
		t.Errorf("zero: %v %q", k, s)
	}
	for _, tt := range []struct {
		name string
		tell func(l *Line)
		kind Kind
		stay time.Duration
	}{
		{"say", func(l *Line) { l.Say(t0, "x") }, Info, InfoFor},
		{"refuse", func(l *Line) { l.Refuse(t0, "x") }, Refusal, RefusalFor},
		{"fail", func(l *Line) { l.Fail(t0, "x") }, Failure, FailureFor},
	} {
		tt.tell(&l)
		last := t0.Add(tt.stay - time.Millisecond)
		if k, s := l.Shown(last); k != tt.kind || s != "x" {
			t.Errorf("%s: %v %q just before its end", tt.name, k, s)
		}
		if due := l.Due(t0); !due.Equal(t0.Add(tt.stay)) {
			t.Errorf("%s: due %v", tt.name, due)
		}
		if k, s := l.Shown(t0.Add(tt.stay)); k != None || s != "" || !l.Due(t0.Add(tt.stay)).IsZero() {
			t.Errorf("%s: %v %q at its end", tt.name, k, s)
		}
		// only a refusal goes at the next key
		l.KeyPressed()
		if k, _ := l.Shown(t0); (k == None) != (tt.kind == Refusal) {
			t.Errorf("%s: %v after a key", tt.name, k)
		}
	}
	// the newest replaces the one before
	l.Fail(t0, "failed")
	l.Say(t0.Add(time.Second), "done")
	if k, s := l.Shown(t0.Add(2 * time.Second)); k != Info || s != "done" {
		t.Errorf("newest: %v %q", k, s)
	}
}
