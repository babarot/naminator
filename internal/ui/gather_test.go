package ui

import (
	"fmt"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// slowSender takes a while for each message, as a busy program does.
type slowSender struct {
	mu   sync.Mutex
	took time.Duration
	msgs []tea.Msg
}

func (s *slowSender) Send(msg tea.Msg) {
	time.Sleep(s.took)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, msg)
}

// Every result is told once, in fewer messages than results, and none is
// left when it is closed.
func TestGatherer(t *testing.T) {
	s := &slowSender{took: 2 * time.Millisecond}
	g := newGatherer(s, time.Millisecond)
	var wg sync.WaitGroup
	const workers, each = 8, 250
	for w := range workers {
		wg.Go(func() {
			for i := range each {
				g.add(exifResultMsg{image: fmt.Sprintf("%d-%d", w, i)})
			}
		})
	}
	wg.Wait()
	g.close()
	told := map[string]int{}
	for _, msg := range s.msgs {
		results, ok := msg.(exifResultsMsg)
		if !ok {
			t.Fatalf("told %T", msg)
		}
		for _, r := range results {
			told[r.image]++
		}
	}
	if len(told) != workers*each {
		t.Errorf("%d results told, want %d", len(told), workers*each)
	}
	for image, n := range told {
		if n != 1 {
			t.Errorf("%s told %d times", image, n)
		}
	}
	if len(s.msgs) > workers*each/4 {
		t.Errorf("%d messages for %d results", len(s.msgs), workers*each)
	}
}

// Closing with nothing gathered ends at once, and a result just added is
// told.
func TestGathererClose(t *testing.T) {
	s := &slowSender{}
	newGatherer(s, time.Hour).close()
	if len(s.msgs) != 0 {
		t.Errorf("told %v", s.msgs)
	}
	g := newGatherer(s, time.Hour)
	g.add(exifResultMsg{image: "a"})
	g.close()
	if len(s.msgs) != 1 {
		t.Errorf("told %d messages, want the one gathered", len(s.msgs))
	}
}

// The spinner turns while photos are read, and stops once they are.
func TestSpinnerStops(t *testing.T) {
	m, photos := newTestTUI(t, 3)
	tick := func() tea.Cmd {
		_, cmd := m.Update(m.spinner.Tick())
		return cmd
	}
	if tick() == nil {
		t.Error("the spinner should turn while photos are read")
	}
	m = m.send(t, exifResultsMsg{{image: photos[0].Path, photo: photos[0]}, {image: photos[1].Path, photo: photos[1]}})
	if m.nRead != 2 || tick() == nil {
		t.Errorf("read %d of 3", m.nRead)
	}
	// told again, a photo is not counted twice
	m = m.send(t, exifResultsMsg{{image: photos[1].Path, photo: photos[1]}, {image: photos[2].Path, photo: photos[2]}})
	if m.nRead != 3 {
		t.Fatalf("read %d of 3", m.nRead)
	}
	if tick() != nil {
		t.Error("the spinner should stop once all are read")
	}
	if m.rows[1].target == "" || m.rows[2].suffix != "_1" {
		t.Errorf("not planned: %+v %+v", m.rows[1], m.rows[2])
	}
}
