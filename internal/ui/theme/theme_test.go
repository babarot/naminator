package theme

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFor(t *testing.T) {
	t.Setenv("NO_COLOR", "") // restores it after the test
	if !For(true).Plain {
		t.Error("NO_COLOR set, also empty, should make the styles plain")
	}
	if err := os.Unsetenv("NO_COLOR"); err != nil {
		t.Fatal(err)
	}
	dark, light := For(true), For(false)
	if dark.Plain || light.Plain {
		t.Error("without NO_COLOR the styles should not be plain")
	}
	if dark.CursorLine("x", false) == light.CursorLine("x", false) {
		t.Error("the cursor's line should suit the background")
	}
}

// The background of the cursor's line goes on past the resets of the
// styles in it.
func TestCursorLine(t *testing.T) {
	st := New(Dark())
	line := st.Warn.Render("a") + " b "
	got := st.CursorLine(line, false)
	bg := "\x1b[48;2;40;52;87m"
	if !strings.HasPrefix(got, bg) || !strings.Contains(got, "m"+bg+" b ") || !strings.HasSuffix(got, "\x1b[49m") || ansi.Strip(got) != "a b " {
		t.Errorf("got %q", got)
	}
	if st.CursorLine(line, true) == got {
		t.Error("editing should have a background of its own")
	}
	if got := Plain().CursorLine("x", false); got != "x" {
		t.Errorf("plain: %q", got)
	}
}

func TestPlain(t *testing.T) {
	st := Plain()
	for _, s := range []string{st.Success.Render("x"), st.Failure.Render("x"), st.Warn.Render("x"), st.Active.Render("x"), st.Path.Render("x"),
		st.Part.Render("x"), st.PartStrong.Render("x"), st.Strong.Render("x"), st.Dim.Render("x")} {
		if s != "x" {
			t.Errorf("plain style gives %q", s)
		}
	}
}
