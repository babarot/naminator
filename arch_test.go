package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// The packages and what each may import, from the bottom layer up. A
// package imports only from lower layers and only the outside modules
// listed for it. docs/architecture.md shows this table, and
// TestArchitectureDoc keeps the two the same. Test files are not checked:
// a test may import what helps it.
//
// A package not in the table fails the test, so a new one is placed here
// on purpose.
var layers = []layer{
	{"internal/photo", 0, nil, []string{"github.com/barasher/go-exiftool", "github.com/gabriel-vasile/mimetype"}, "Photos: their EXIF data, read with exiftool, and the files that are photos"},
	{"internal/rename", 1, []string{"internal/photo"}, nil, "Where photos go: templates, paths no two photos share, sidecars, and the directories left empty"},
	{"internal/config", 2, []string{"internal/rename"}, []string{"github.com/goccy/go-yaml"}, "The configuration file"},
	{"internal/ui/keymap", 3, nil, nil, "Keys bound to actions by name"},
	{"internal/ui/statusline", 3, nil, nil, "What the status line shows, for a moment"},
	{"internal/ui/theme", 3, nil, []string{"charm.land/lipgloss/v2"}, "The colors, by what they are for, and their styles"},
	{"internal/ui", 4, []string{"internal/config", "internal/photo", "internal/rename", "internal/ui/keymap", "internal/ui/statusline", "internal/ui/theme"}, []string{"charm.land/bubbles/v2", "charm.land/bubbletea/v2", "github.com/charmbracelet/x/ansi", "github.com/mattn/go-isatty", "golang.org/x/sys", "golang.org/x/term"}, "The interactive screen and the display of a run"},
	{"internal/cli", 5, []string{"internal/config", "internal/photo", "internal/rename", "internal/ui"}, []string{"github.com/jessevdk/go-flags", "github.com/mattn/go-isatty", "github.com/nxadm/tail"}, "The command line: the flags, the workers, and moving the files"},
	{"", 6, []string{"internal/cli"}, nil, "Calls cli.Main with the version"},
}

// layer is a package's place: its layer, the packages of this module
// (named by their path in it) and outside modules it may import, and what
// it does.
type layer struct {
	pkg      string
	layer    int
	local    []string
	external []string
	does     string
}

const module = "github.com/babarot/naminator"

type goPackage struct {
	ImportPath string
	Imports    []string
}

// releaseOS are the systems a release is built for. What a package imports can
// differ between them, as the lock does.
var releaseOS = []string{"darwin", "linux", "windows"}

// listPackages are the packages, each with what it imports on any of releaseOS.
func listPackages(t *testing.T) []goPackage {
	t.Helper()
	var pkgs []goPackage
	for _, goos := range releaseOS {
		cmd := exec.Command("go", "list", "-json", "./...")
		cmd.Env = append(os.Environ(), "GOOS="+goos)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("go list: %v", err)
		}
		dec := json.NewDecoder(bytes.NewReader(out))
		for {
			var p goPackage
			if err := dec.Decode(&p); err == io.EOF {
				break
			} else if err != nil {
				t.Fatal(err)
			}
			i := slices.IndexFunc(pkgs, func(q goPackage) bool { return q.ImportPath == p.ImportPath })
			if i < 0 {
				pkgs = append(pkgs, p)
				continue
			}
			for _, imp := range p.Imports {
				if !slices.Contains(pkgs[i].Imports, imp) {
					pkgs[i].Imports = append(pkgs[i].Imports, imp)
				}
			}
		}
	}
	return pkgs
}

func TestArchitecture(t *testing.T) {
	for _, p := range listPackages(t) {
		rel := strings.TrimPrefix(strings.TrimPrefix(p.ImportPath, module), "/")
		i := slices.IndexFunc(layers, func(l layer) bool { return l.pkg == rel })
		if i < 0 {
			t.Errorf("%s is not in the layer table in arch_test.go and docs/architecture.md", p.ImportPath)
			continue
		}
		l := layers[i]
		for _, imp := range p.Imports {
			switch {
			case strings.HasPrefix(imp, module+"/"):
				dep := strings.TrimPrefix(imp, module+"/")
				if !slices.Contains(l.local, dep) {
					t.Errorf("%s imports %s, which is not in its allowed imports", p.ImportPath, imp)
				}
			case strings.Contains(strings.SplitN(imp, "/", 2)[0], "."):
				if !slices.ContainsFunc(l.external, func(m string) bool { return imp == m || strings.HasPrefix(imp, m+"/") }) {
					t.Errorf("%s imports %s, an outside module not allowed for it", p.ImportPath, imp)
				}
			}
		}
	}
}

// TestLayersGoDownward checks the table itself: every allowed import is a
// package of a lower layer.
func TestLayersGoDownward(t *testing.T) {
	layerOf := map[string]int{}
	for _, l := range layers {
		layerOf[l.pkg] = l.layer
	}
	for _, l := range layers {
		for _, dep := range l.local {
			dl, ok := layerOf[dep]
			if !ok || dl >= l.layer {
				t.Errorf("%s (layer %d) may import %s (layer %d, known %v)", l.pkg, l.layer, dep, dl, ok)
			}
		}
	}
}

// TestNoUnusedImportAllowed checks that every package of the table exists
// and every import it allows is made, so the table tells what each
// package depends on. An outside
// module is used when the module or a package in it is imported.
func TestNoUnusedImportAllowed(t *testing.T) {
	imports := map[string][]string{}
	for _, p := range listPackages(t) {
		rel := strings.TrimPrefix(strings.TrimPrefix(p.ImportPath, module), "/")
		imports[rel] = p.Imports
	}
	for _, l := range layers {
		imps, ok := imports[l.pkg]
		if !ok {
			t.Errorf("%s is in the table but is not a package", pkgName(l.pkg))
			continue
		}
		for _, dep := range l.local {
			if !slices.Contains(imps, module+"/"+dep) {
				t.Errorf("%s may import %s but does not; remove it from the table", pkgName(l.pkg), dep)
			}
		}
		for _, m := range l.external {
			if !slices.ContainsFunc(imps, func(imp string) bool { return imp == m || strings.HasPrefix(imp, m+"/") }) {
				t.Errorf("%s may import %s but does not; remove it from the table", pkgName(l.pkg), m)
			}
		}
	}
}

// pkgName is a package of the table as people call it.
func pkgName(pkg string) string {
	if pkg == "" {
		return "main"
	}
	return pkg
}

var update = flag.Bool("update", false, "rewrite the layer table in docs/architecture.md")

const (
	docPath  = "docs/architecture.md"
	docStart = "<!-- layers:start -->"
	docEnd   = "<!-- layers:end -->"
)

// docTable is the layer table as docs/architecture.md shows it.
func docTable() string {
	var b strings.Builder
	b.WriteString("| Layer | Package | Does | May import | Outside modules |\n|---|---|---|---|---|\n")
	for _, l := range layers {
		name := "`" + l.pkg + "`"
		if l.pkg == "" {
			name = "`main`"
		}
		join := func(ss []string) string {
			if len(ss) == 0 {
				return "-"
			}
			q := make([]string, len(ss))
			for i, s := range ss {
				q[i] = "`" + s + "`"
			}
			return strings.Join(q, ", ")
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s |\n", l.layer, name, l.does, join(l.local), join(l.external))
	}
	return b.String()
}

func TestArchitectureDoc(t *testing.T) {
	src, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(src)
	start, end := strings.Index(doc, docStart), strings.Index(doc, docEnd)
	if start < 0 || end < start {
		t.Fatalf("%s has no %s ... %s block", docPath, docStart, docEnd)
	}
	want := docStart + "\n" + docTable() + docEnd
	got := doc[start : end+len(docEnd)]
	if got == want {
		return
	}
	if *update {
		if err := os.WriteFile(docPath, []byte(doc[:start]+want+doc[end+len(docEnd):]), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Errorf("the layer table in %s differs from arch_test.go; run go test -run TestArchitectureDoc -update", docPath)
}
