package output

const Limit = 64 * 1024

// Tail retains the last Limit bytes written to it. Its zero value is ready to use.
// It is not safe for concurrent use.
type Tail struct {
	data      [Limit]byte
	length    int
	truncated bool
}

func (t *Tail) Write(p []byte) (int, error) {
	n := len(p)
	if n >= Limit {
		t.truncated = t.truncated || t.length+n > Limit
		copy(t.data[:], p[n-Limit:])
		t.length = Limit
		return n, nil
	}
	if overflow := t.length + n - Limit; overflow > 0 {
		copy(t.data[:], t.data[overflow:t.length])
		t.length -= overflow
		t.truncated = true
	}
	copy(t.data[t.length:], p)
	t.length += n
	return n, nil
}

func (t *Tail) String() string {
	text := string(t.data[:t.length])
	if t.truncated {
		return "[output truncated]\n" + text
	}
	return text
}
