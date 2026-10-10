package main

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// mockResultMsg implements resultMsg for testing
type mockResultMsg struct {
	path string
	err  error
	str  string
}

func (m mockResultMsg) Path() string  { return m.path }
func (m mockResultMsg) Err() error    { return m.err }
func (m mockResultMsg) String() string { return m.str }

func TestReplaceFirstNilOrAppend(t *testing.T) {
	t.Run("replaces first nil", func(t *testing.T) {
		m := &model{
			results: []resultMsg{nil, nil, nil},
			height:  3,
		}
		msg := mockResultMsg{path: "a.jpg", str: "a"}
		m.replaceFirstNilOrAppend(msg)

		if m.results[0] == nil {
			t.Error("first element should not be nil")
		}
		if m.results[0].Path() != "a.jpg" {
			t.Errorf("got path %q, want %q", m.results[0].Path(), "a.jpg")
		}
		// second and third should still be nil
		if m.results[1] != nil || m.results[2] != nil {
			t.Error("only first nil should be replaced")
		}
	})

	t.Run("replaces second nil when first is filled", func(t *testing.T) {
		first := mockResultMsg{path: "a.jpg", str: "a"}
		m := &model{
			results: []resultMsg{first, nil, nil},
			height:  3,
		}
		msg := mockResultMsg{path: "b.jpg", str: "b"}
		m.replaceFirstNilOrAppend(msg)

		if m.results[1] == nil || m.results[1].Path() != "b.jpg" {
			t.Error("second element should be b.jpg")
		}
	})

	t.Run("shifts and appends when all filled", func(t *testing.T) {
		m := &model{
			results: []resultMsg{
				mockResultMsg{path: "a.jpg"},
				mockResultMsg{path: "b.jpg"},
				mockResultMsg{path: "c.jpg"},
			},
			height: 3,
		}
		msg := mockResultMsg{path: "d.jpg"}
		m.replaceFirstNilOrAppend(msg)

		if len(m.results) != 3 {
			t.Fatalf("len = %d, want 3", len(m.results))
		}
		if m.results[0].Path() != "b.jpg" {
			t.Errorf("first = %q, want b.jpg", m.results[0].Path())
		}
		if m.results[2].Path() != "d.jpg" {
			t.Errorf("last = %q, want d.jpg", m.results[2].Path())
		}
	})
}

func TestTrimNonErrorMessageAndAppend(t *testing.T) {
	t.Run("removes first non-error and appends", func(t *testing.T) {
		m := &model{
			results: []resultMsg{
				mockResultMsg{path: "a.jpg"},
				mockResultMsg{path: "b.jpg", err: errors.New("fail")},
				mockResultMsg{path: "c.jpg"},
			},
			height: 3,
		}
		msg := mockResultMsg{path: "d.jpg"}
		m.trimNonErrorMessageAndAppend(msg)

		// "a.jpg" (non-error) should be removed, error "b.jpg" retained
		paths := make([]string, len(m.results))
		for i, r := range m.results {
			paths[i] = r.Path()
		}
		if len(m.results) != 3 {
			t.Fatalf("len = %d, want 3, got %v", len(m.results), paths)
		}
		// b.jpg (error) should be retained
		hasError := false
		for _, r := range m.results {
			if r.Path() == "b.jpg" {
				hasError = true
			}
		}
		if !hasError {
			t.Errorf("error message b.jpg should be retained, got %v", paths)
		}
	})

	t.Run("removes nil first", func(t *testing.T) {
		m := &model{
			results: []resultMsg{
				nil,
				mockResultMsg{path: "a.jpg", err: errors.New("fail")},
			},
			height: 3,
		}
		msg := mockResultMsg{path: "b.jpg"}
		m.trimNonErrorMessageAndAppend(msg)

		// nil should be removed
		for _, r := range m.results {
			if r == nil {
				t.Error("nil should have been removed")
			}
		}
	})

	t.Run("respects height limit", func(t *testing.T) {
		m := &model{
			results: []resultMsg{
				mockResultMsg{path: "a.jpg", err: errors.New("e1")},
				mockResultMsg{path: "b.jpg", err: errors.New("e2")},
				mockResultMsg{path: "c.jpg", err: errors.New("e3")},
			},
			height: 3,
		}
		msg := mockResultMsg{path: "d.jpg", err: errors.New("e4")}
		m.trimNonErrorMessageAndAppend(msg)

		if len(m.results) > m.height {
			t.Errorf("len = %d, should not exceed height %d", len(m.results), m.height)
		}
	})
}

func TestDecideProgress(t *testing.T) {
	newTestModel := func(done int, elapsed time.Duration) model {
		m := model{files: map[string]state{}, total: 100, startTime: time.Now().Add(-elapsed)}
		for i := range done {
			m.files[fmt.Sprint(i)] = succeeded
		}
		return m
	}

	t.Run("undecided before 3 seconds", func(t *testing.T) {
		m := newTestModel(0, time.Second)
		m.decideProgress()
		if m.showProgressSet {
			t.Error("should not decide before 3 seconds")
		}
	})

	t.Run("shown when less than a quarter is done", func(t *testing.T) {
		m := newTestModel(10, 4*time.Second)
		m.decideProgress()
		if !m.showProgressSet || !m.showProgress {
			t.Errorf("showProgressSet=%v showProgress=%v, want true, true", m.showProgressSet, m.showProgress)
		}
	})

	t.Run("hidden when a quarter or more is done, and kept", func(t *testing.T) {
		m := newTestModel(50, 4*time.Second)
		m.decideProgress()
		if !m.showProgressSet || m.showProgress {
			t.Errorf("showProgressSet=%v showProgress=%v, want true, false", m.showProgressSet, m.showProgress)
		}
		m.files = map[string]state{}
		m.decideProgress()
		if m.showProgress {
			t.Error("the decision should not change once made")
		}
	})
}
