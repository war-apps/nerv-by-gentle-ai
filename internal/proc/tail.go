package proc

// tailBuffer is an io.Writer that keeps only the last limit bytes written
// to it. It grows to at most twice the limit before compacting, so a long
// stream of small writes costs amortised constant time per byte.
type tailBuffer struct {
	limit int
	buf   []byte
}

func newTailBuffer(limit int) *tailBuffer { return &tailBuffer{limit: limit} }

// Write implements io.Writer; it never fails.
func (t *tailBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if n >= t.limit {
		t.buf = append(t.buf[:0], p[n-t.limit:]...)
		return n, nil
	}
	t.buf = append(t.buf, p...)
	if len(t.buf) > 2*t.limit {
		t.buf = append(t.buf[:0], t.buf[len(t.buf)-t.limit:]...)
	}
	return n, nil
}

// Bytes returns the last limit bytes written (fewer if less was written).
func (t *tailBuffer) Bytes() []byte {
	if len(t.buf) > t.limit {
		return t.buf[len(t.buf)-t.limit:]
	}
	return t.buf
}
