package adapters

import (
	"io"
	"strings"
	"sync"
)

// secretOutput removes a resolved credential before child output reaches disk.
// A suffix that could be the beginning of the credential is held across writes.
type secretOutput struct {
	mu      sync.Mutex
	dst     io.Writer
	secret  string
	pending string
}

func (w *secretOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending += string(p)
	if err := w.flush(false); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *secretOutput) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.flush(true)
}

func (w *secretOutput) flush(final bool) error {
	var out strings.Builder
	for len(w.pending) > 0 {
		if w.secret != "" && strings.HasPrefix(w.pending, w.secret) {
			out.WriteString("[REDACTED]")
			w.pending = w.pending[len(w.secret):]
		} else if !final && w.secret != "" && strings.HasPrefix(w.secret, w.pending) {
			break
		} else {
			out.WriteByte(w.pending[0])
			w.pending = w.pending[1:]
		}
	}
	_, err := io.WriteString(w.dst, out.String())
	return err
}

// Keep credentials out of command lines, including process listings and errors.
func withHFToken(env []string, token string) []string {
	result := make([]string, 0, len(env)+1)
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(name, "HF_TOKEN") {
			result = append(result, entry)
		}
	}
	return append(result, "HF_TOKEN="+token)
}
