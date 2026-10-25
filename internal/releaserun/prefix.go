package releaserun

import (
	"bytes"
	"io"
	"sync"
)

// prefixWriter writes complete lines to out, each preceded by prefix. A
// partial line is held until its newline, Flush, or until it reaches max
// bytes (then it is written as a line of its own). Each line is one Write
// to out, so lines from several steps never interleave mid-line on a
// writer that serialises its writes.
//
// Write never fails: an error of out is kept in err and later lines are
// dropped, so a closed terminal cannot block or kill the script (the
// process keeps a drained pipe).
type prefixWriter struct {
	mu     sync.Mutex
	out    io.Writer
	prefix []byte
	max    int
	line   []byte
	err    error
}

func newPrefixWriter(out io.Writer, prefix string, max int) *prefixWriter {
	return &prefixWriter{out: out, prefix: []byte(prefix), max: max}
}

// Write implements io.Writer.
func (w *prefixWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := len(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		seg := p
		if i >= 0 {
			seg = p[:i+1]
		}
		room := w.max - len(w.line)
		if content := len(seg) - boolInt(i >= 0); content > room {
			// The line outgrows max before its newline: cut it here.
			w.line = append(w.line, p[:room]...)
			p = p[room:]
			w.emit()
			continue
		}
		w.line = append(w.line, seg...)
		p = p[len(seg):]
		if i >= 0 {
			w.emit()
		}
	}
	return n, nil
}

// Flush writes a pending partial line, ended with a newline.
func (w *prefixWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.line) > 0 {
		w.emit()
	}
}

// emit writes the held line with its prefix and a newline if it has none.
func (w *prefixWriter) emit() {
	buf := make([]byte, 0, len(w.prefix)+len(w.line)+1)
	buf = append(append(buf, w.prefix...), w.line...)
	if !bytes.HasSuffix(w.line, []byte("\n")) {
		buf = append(buf, '\n')
	}
	w.line = w.line[:0]
	if w.err != nil {
		return
	}
	if _, err := w.out.Write(buf); err != nil {
		w.err = err
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
