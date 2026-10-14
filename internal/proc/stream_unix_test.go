//go:build !windows

package proc

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// signalWriter records what it receives and signals the first write.
type signalWriter struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	first chan struct{}
	once  sync.Once
}

func newSignalWriter() *signalWriter { return &signalWriter{first: make(chan struct{})} }

func (w *signalWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	w.once.Do(func() { close(w.first) })
	return len(p), nil
}

func (w *signalWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// The writer sees the output while the process is still running, not when
// it ends.
func TestExecRunner_StreamsOutputLive(t *testing.T) {
	w := newSignalWriter()
	done := make(chan error, 1)
	go func() {
		_, err := ExecRunner{}.Run(context.Background(), Command{
			Name: "sh", Args: []string{"-c", "echo first; sleep 2; echo second"},
			Stdout: w, Stderr: w,
		})
		done <- err
	}()
	select {
	case <-w.first:
	case err := <-done:
		t.Fatalf("Run returned (%v) before the writer saw any output", err)
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("the writer saw nothing while the process ran")
	}
	select {
	case err := <-done:
		t.Fatalf("Run returned (%v) right after the first line: output was not live", err)
	default:
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := w.String(); got != "first\nsecond\n" {
		t.Errorf("streamed %q", got)
	}
}

// One writer for both streams gives the process one pipe, so stdout and
// stderr lines arrive in the order the process wrote them, and the result
// holds that combined output.
func TestExecRunner_SameWriterKeepsTheOrderOfBothStreams(t *testing.T) {
	var script strings.Builder
	var want strings.Builder
	for i := range 50 {
		stream := ""
		if i%2 == 1 {
			stream = " >&2"
		}
		fmt.Fprintf(&script, "echo line%d%s\n", i, stream)
		fmt.Fprintf(&want, "line%d\n", i)
	}
	w := newSignalWriter()
	res, err := ExecRunner{}.Run(context.Background(), Command{Name: "sh", Args: []string{"-c", script.String()}, Stdout: w, Stderr: w})
	if err != nil {
		t.Fatal(err)
	}
	if got := w.String(); got != want.String() {
		t.Errorf("streamed:\n%s\nwant:\n%s", got, want.String())
	}
	if string(res.Stdout) != want.String() || len(res.Stderr) != 0 {
		t.Errorf("result stdout %q, stderr %q; want the combined output in stdout", res.Stdout, res.Stderr)
	}
}

// Separate writers receive their own stream; the result keeps each tail.
func TestExecRunner_SeparateWriters(t *testing.T) {
	out, errw := newSignalWriter(), newSignalWriter()
	res, err := ExecRunner{}.Run(context.Background(), Command{Name: "sh", Args: []string{"-c", "echo out; echo err >&2"}, Stdout: out, Stderr: errw})
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "out\n" || errw.String() != "err\n" {
		t.Errorf("stdout writer %q, stderr writer %q", out.String(), errw.String())
	}
	if string(res.Stdout) != "out\n" || string(res.Stderr) != "err\n" {
		t.Errorf("result stdout %q, stderr %q", res.Stdout, res.Stderr)
	}
}

// A streamed command keeps only the last TailLimit bytes in its result,
// while the writer gets everything.
func TestExecRunner_StreamedResultIsABoundedTail(t *testing.T) {
	w := newSignalWriter()
	res, err := ExecRunner{}.Run(context.Background(), Command{
		Name: "sh", Args: []string{"-c", `i=0; while [ $i -lt 200 ]; do echo line$i; i=$((i+1)); done`},
		Stdout: w, Stderr: w, TailLimit: 16,
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(w.String(), "\n"); n != 200 {
		t.Errorf("writer got %d lines, want 200", n)
	}
	if len(res.Stdout) > 16 || !strings.HasSuffix(string(res.Stdout), "line199\n") {
		t.Errorf("result tail %q, want at most 16 bytes ending in line199", res.Stdout)
	}
}

// Without writers the whole output is captured, as before streaming.
func TestExecRunner_UnstreamedOutputIsComplete(t *testing.T) {
	res, err := ExecRunner{}.Run(context.Background(), Command{Name: "sh", Args: []string{"-c", `head -c 200000 /dev/zero | tr '\0' x`}, TailLimit: 16})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Stdout) != 200000 {
		t.Errorf("captured %d bytes, want 200000", len(res.Stdout))
	}
}

// Dir and Env are applied to the process.
func TestExecRunner_DirAndEnv(t *testing.T) {
	dir := t.TempDir()
	res, err := ExecRunner{}.Run(context.Background(), Command{Name: "sh", Args: []string{"-c", `pwd; echo "$NERV_PROC_TEST"`}, Dir: dir, Env: []string{"PATH=/usr/bin:/bin", "NERV_PROC_TEST=hello"}})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(res.Stdout)), "\n")
	if len(lines) != 2 || !strings.HasSuffix(lines[0], strings.TrimPrefix(dir, "/private")) || lines[1] != "hello" {
		t.Errorf("output %q, want the dir %s and hello", res.Stdout, dir)
	}
}
