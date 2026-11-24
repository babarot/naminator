package ui

import (
	"sync"
	"time"
)

// gatherEvery is how long the results of the reading are gathered before the
// screen is told of them: the reading is faster than the screen can plan and
// draw for each.
const gatherEvery = 50 * time.Millisecond

// gatherer tells the results of the reading to a program together, at most
// once in a while. While the program is busy with those it was told, more
// are gathered, so the reading never waits for it.
type gatherer struct {
	to    sender
	every time.Duration

	mu      sync.Mutex
	pending []exifResultMsg

	wake    chan struct{} // something is pending
	closing chan struct{} // no more will come
	closed  chan struct{} // everything was told
}

func newGatherer(to sender, every time.Duration) *gatherer {
	g := &gatherer{to: to, every: every, wake: make(chan struct{}, 1), closing: make(chan struct{}), closed: make(chan struct{})}
	go g.run()
	return g
}

// add gathers r. It does not wait.
func (g *gatherer) add(r exifResultMsg) {
	g.mu.Lock()
	g.pending = append(g.pending, r)
	g.mu.Unlock()
	select {
	case g.wake <- struct{}{}:
	default:
	}
}

// close tells what is left and returns once it was. Nothing is to be added
// after.
func (g *gatherer) close() {
	close(g.closing)
	<-g.closed
}

func (g *gatherer) run() {
	defer close(g.closed)
	for {
		select {
		case <-g.wake:
			select {
			case <-time.After(g.every):
			case <-g.closing:
			}
			g.tell()
		case <-g.closing:
			g.tell()
			return
		}
	}
}

func (g *gatherer) tell() {
	g.mu.Lock()
	results := g.pending
	g.pending = nil
	g.mu.Unlock()
	if len(results) > 0 {
		g.to.Send(exifResultsMsg(results))
	}
}
