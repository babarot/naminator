package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/babarot/naminator/internal/ui/keymap"

	"github.com/charmbracelet/x/ansi"
)

// In no state is a key two actions, and every scope is read in some state.
func TestKeysDoNotConflict(t *testing.T) {
	read := map[keymap.Scope]bool{}
	for s := range numUIStates {
		bound := map[string]keymap.Action{}
		for _, scope := range s.scopes() {
			read[scope] = true
			for _, d := range keymap.Defaults {
				if d.Scope != scope {
					continue
				}
				for _, k := range d.Keys {
					if other, ok := bound[k]; ok {
						t.Errorf("state %q: %q is both %s and %s", s, k, other, d.Action)
					}
					bound[k] = d.Action
				}
			}
		}
	}
	for _, d := range keymap.Defaults {
		if !read[d.Scope] {
			t.Errorf("%s: scope %d is read in no state", d.Action, d.Scope)
		}
	}
}

// sources are the files of the package but the tests.
func sources(t *testing.T) (*token.FileSet, []*ast.File) {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	return fset, files
}

// No key is compared in the code of the screens: they ask the keymap.
func TestNoKeyIsCompared(t *testing.T) {
	isKey := map[string]bool{}
	for _, d := range keymap.Defaults {
		for _, k := range d.Keys {
			isKey[k] = true
		}
	}
	fset, files := sources(t)
	check := func(e ast.Expr) {
		lit, ok := e.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return
		}
		if s, err := strconv.Unquote(lit.Value); err == nil && isKey[s] {
			t.Errorf("%s compares the key %q: ask the keymap which action it is", fset.Position(lit.Pos()), s)
		}
	}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CaseClause:
				for _, e := range n.List {
					check(e)
				}
			case *ast.BinaryExpr:
				if n.Op == token.EQL || n.Op == token.NEQ {
					check(n.X)
					check(n.Y)
				}
			}
			return true
		})
	}
}

// Every action that has keys is handled by the screens.
func TestEveryActionIsHandled(t *testing.T) {
	handled := map[string]bool{}
	_, files := sources(t)
	name := func(e ast.Expr) {
		if sel, ok := e.(*ast.SelectorExpr); ok {
			if x, ok := sel.X.(*ast.Ident); ok && x.Name == "keymap" {
				handled[sel.Sel.Name] = true
			}
		}
	}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CaseClause:
				for _, e := range n.List {
					name(e)
				}
			case *ast.BinaryExpr:
				name(n.X)
				name(n.Y)
			}
			return true
		})
	}
	for _, d := range keymap.Defaults {
		// the constant of "page_up" is PageUp
		var c strings.Builder
		for part := range strings.SplitSeq(string(d.Action), "_") {
			c.WriteString(strings.ToUpper(part[:1]) + part[1:])
		}
		if !handled[c.String()] {
			t.Errorf("no screen handles %s", d.Action)
		}
	}
}

// The key line fits the terminal: what does not fit is left out, but the
// keys that rename and quit.
func TestKeyLineFits(t *testing.T) {
	for _, s := range []uiState{stateList, stateTree, stateEdit} {
		m := states(t)[s]
		for _, w := range []int{200, 100, 80, 60, 40, 30} {
			m.width = w
			line := ansi.Strip(m.help())
			if got := ansi.StringWidth(line); got > w-2 {
				t.Errorf("state %q on %d columns: %d cells: %q", s, w, got, line)
			}
			keep := []string{"enter rename", "q quit"}
			if s == stateEdit {
				keep = []string{"esc/enter done"}
			}
			for _, k := range keep {
				if !strings.Contains(line, k) {
					t.Errorf("state %q on %d columns: %q lacks %q", s, w, line, k)
				}
			}
		}
	}
}
