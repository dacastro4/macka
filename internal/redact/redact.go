// Package redact strips secret values from anything written to logs or the
// terminal (X-10, T-5).
package redact

import (
	"io"
	"strings"
	"sync"
)

// Mask replaces each secret.
const Mask = "[REDACTED]"

// Writer replaces registered secrets before passing bytes on.
type Writer struct {
	mu      sync.Mutex
	out     io.Writer
	secrets []string
}

// NewWriter wraps out.
func NewWriter(out io.Writer) *Writer { return &Writer{out: out} }

// Add registers secret values. Empty and very short values are ignored so
// redaction never mangles ordinary output.
func (w *Writer) Add(values ...string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, v := range values {
		if len(v) >= 8 {
			w.secrets = append(w.secrets, v)
		}
	}
}

// Write redacts p and writes it. It reports len(p) on success so callers
// treat the write as complete even though the redacted length differs.
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	s := string(p)
	for _, sec := range w.secrets {
		s = strings.ReplaceAll(s, sec, Mask)
	}
	w.mu.Unlock()
	if _, err := io.WriteString(w.out, s); err != nil {
		return 0, err
	}
	return len(p), nil
}
