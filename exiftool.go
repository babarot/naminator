package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// exiftoolTags are the tags naminator reads (see getExifdata). Asking
// exiftool for these alone, rather than for every tag, makes it do less
// work per photo and print a small fraction of the JSON. SourceFile comes
// with every answer; SubSecDateTimeOriginal is a composite tag, which
// exiftool still builds from the tags it is made of.
var exiftoolTags = []string{
	"-FileName",
	"-FileTypeExtension",
	"-SubSecDateTimeOriginal",
	"-DateTimeOriginal",
	"-Model",
}

// exiftoolPath is the exiftool command that is run; tests replace it.
var exiftoolPath = func() string {
	if runtime.GOOS == "windows" {
		return "exiftool.exe"
	}
	return "exiftool"
}()

// exiftoolCloseTimeout is how long Close waits for exiftool to exit before
// it kills it.
var exiftoolCloseTimeout = time.Second

// readyToken is what exiftool prints when it has answered a command: on
// stdout itself, and on stderr because of -echo4, so that both streams can
// be read up to the end of one answer.
const readyToken = "{ready}"

// errKeyNotFound is the error for a tag that exiftool did not report.
var errKeyNotFound = errors.New("key not found")

// exiftool is one exiftool process kept running with -stay_open, reading a
// command per file from its stdin. Calls on one process run one at a time;
// run several processes to read photos in parallel.
type exiftool struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr *bufio.Reader
	// err is set once the process can no longer be used
	err error
}

// startExiftool starts an exiftool process. It returns at once: exiftool
// loads in the background, and the first Extract waits for it.
func startExiftool() (*exiftool, error) {
	args := []string{"-stay_open", "True", "-@", "-", "-common_args", "-json", "-echo4", readyToken}
	args = append(args, exiftoolTags...)
	cmd := exec.Command(exiftoolPath, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to run exiftool: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to run exiftool: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to run exiftool: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to run exiftool: %w", err)
	}
	return &exiftool{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
		stderr: bufio.NewReader(stderr),
	}, nil
}

// Extract reads the metadata of the file at path.
func (e *exiftool) Extract(path string) (Photo, error) {
	m, err := e.metadata(path)
	if err != nil {
		return Photo{Name: filepath.Base(path), Path: path}, err
	}
	return photoFromMetadata(path, m)
}

// metadata asks exiftool for the tags of the file at path.
func (e *exiftool) metadata(path string) (fileMetadata, error) {
	s, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if s.IsDir() {
		return nil, errors.New("can't extract metadata from folder")
	}
	// exiftool reads one argument per line, and one starting with "-" as
	// an option
	if strings.ContainsAny(path, "\r\n") {
		return nil, errors.New("exiftool cannot read a path with a line break")
	}
	if strings.HasPrefix(path, "-") {
		path = "." + string(os.PathSeparator) + path
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.err != nil {
		return nil, e.err
	}
	if _, err := io.WriteString(e.stdin, path+"\n-execute\n"); err != nil {
		e.err = fmt.Errorf("exiftool is not running: %w", err)
		return nil, e.err
	}
	out, err := readUntilReady(e.stdout)
	if err != nil {
		e.err = fmt.Errorf("exiftool is not running: %w", err)
		return nil, e.err
	}
	msg, err := readUntilReady(e.stderr)
	if err != nil {
		e.err = fmt.Errorf("exiftool is not running: %w", err)
		return nil, e.err
	}
	return parseExiftoolJSON(out, msg)
}

// Close stops the process.
func (e *exiftool) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.err == nil {
		e.err = errors.New("exiftool is closed")
	}
	_, _ = io.WriteString(e.stdin, "-stay_open\nFalse\n-execute\n")
	_ = e.stdin.Close()
	var err error
	done := make(chan struct{})
	go func() {
		err = e.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(exiftoolCloseTimeout):
		_ = e.cmd.Process.Kill()
		<-done
		return errors.New("timed out waiting for exiftool to exit")
	}
	// The command that stops exiftool also gets the common arguments, so it
	// fails for want of a file: how exiftool exits says nothing
	if _, ok := errors.AsType[*exec.ExitError](err); ok {
		return nil
	}
	return err
}

// readUntilReady reads r up to the next line that is readyToken, and returns
// what came before it.
func readUntilReady(r *bufio.Reader) ([]byte, error) {
	var buf []byte
	for {
		line, err := r.ReadBytes('\n')
		if bytes.Equal(bytes.TrimRight(line, "\r\n"), []byte(readyToken)) {
			return buf, nil
		}
		buf = append(buf, line...)
		if err != nil {
			return buf, err
		}
	}
}

// parseExiftoolJSON parses exiftool's answer about one file: out is what
// it printed on stdout, and msg on stderr.
func parseExiftoolJSON(out, msg []byte) (fileMetadata, error) {
	msg = bytes.TrimSpace(msg)
	if len(bytes.TrimSpace(out)) == 0 {
		if len(msg) > 0 {
			return nil, fmt.Errorf("exiftool: %s", msg)
		}
		return nil, errors.New("failed to extract metadata")
	}
	d := json.NewDecoder(bytes.NewReader(out))
	d.UseNumber()
	var ms []fileMetadata
	if err := d.Decode(&ms); err != nil {
		return nil, fmt.Errorf("failed to read exiftool's answer (%s): %w", out, err)
	}
	if len(ms) == 0 {
		return nil, errors.New("failed to extract metadata")
	}
	m := ms[0]
	if s, err := m.GetString("Error"); err == nil {
		return m, fmt.Errorf("exiftool: Error: %s", s)
	}
	return m, nil
}

// fileMetadata is exiftool's answer about one file, by tag name.
type fileMetadata map[string]any

// GetString returns the tag k as a string, or errKeyNotFound.
func (m fileMetadata) GetString(k string) (string, error) {
	v, ok := m[k]
	if !ok || v == nil {
		return "", errKeyNotFound
	}
	if s, ok := v.(string); ok {
		return s, nil
	}
	return fmt.Sprint(v), nil
}
