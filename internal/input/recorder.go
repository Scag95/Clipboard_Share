package input

import (
	"fmt"
	"strings"
	"sync"
)

// Recorder is an Injector that records events instead of injecting them. It
// backs non-Windows development and tests; the recorded strings use a stable
// canonical format.
type Recorder struct {
	mu   sync.Mutex
	logs []string
}

func (r *Recorder) Move(dx, dy int) error {
	r.record(fmt.Sprintf("move %d %d", dx, dy))
	return nil
}

func (r *Recorder) Scroll(dx, dy int) error {
	r.record(fmt.Sprintf("scroll %d %d", dx, dy))
	return nil
}

func (r *Recorder) Click(button, mode string) error {
	r.record(fmt.Sprintf("click %s %s", button, mode))
	return nil
}

func (r *Recorder) Key(key string, mods []string) error {
	r.record("key " + key + " " + strings.Join(mods, "+"))
	return nil
}

func (r *Recorder) Text(chars string) error {
	r.record("text " + chars)
	return nil
}

func (r *Recorder) record(s string) {
	r.mu.Lock()
	r.logs = append(r.logs, s)
	r.mu.Unlock()
}

// Entries returns a copy of everything recorded so far.
func (r *Recorder) Entries() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.logs))
	copy(out, r.logs)
	return out
}
