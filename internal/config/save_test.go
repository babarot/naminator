package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "naminator", "config.yaml")
	if err := SaveFormat(path, "{ext}/{date}"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "format: \"{ext}/{date}\"\n" {
		t.Errorf("new file = %q", b)
	}
	// the format line is replaced; comments and the rest are kept
	if err := os.WriteFile(path, []byte("# mine\nclean: true\nformat: \"{date}\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := SaveFormat(path, "{year}/{date}"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "# mine\nclean: true\nformat: \"{year}/{date}\"\n" {
		t.Errorf("replaced = %q", b)
	}
	if c, err := Load(path); err != nil || c.Format != "{year}/{date}" || !c.Clean {
		t.Errorf("config = %+v, %v", c, err)
	}
}
