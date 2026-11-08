package match

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
)

// Logger appends one JSON object per line to a file. It is safe for
// concurrent use.
type Logger struct {
	mu  sync.Mutex
	f   *os.File
	w   *bufio.Writer
	err error
}

// NewLogger creates (or truncates) the log file at path.
func NewLogger(path string) (*Logger, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &Logger{f: f, w: bufio.NewWriter(f)}, nil
}

// Log writes v as one line. The first write error is kept and returned by
// Close; later writes are dropped.
func (l *Logger) Log(v any) {
	if l == nil {
		return
	}
	b, err := json.Marshal(v)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil {
		return
	}
	if err != nil {
		l.err = err
		return
	}
	b = append(b, '\n')
	_, l.err = l.w.Write(b)
}

// Flush writes buffered lines to the file.
func (l *Logger) Flush() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err == nil {
		l.err = l.w.Flush()
	}
	return l.err
}

// Close flushes, syncs and closes the file.
func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	err := l.Flush()
	if serr := l.f.Sync(); err == nil {
		err = serr
	}
	if cerr := l.f.Close(); err == nil {
		err = cerr
	}
	return err
}
