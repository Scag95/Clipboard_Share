//go:build !windows

package input

// New returns the platform injector. Off Windows (development, tests) it
// records events instead of injecting them (S7).
func New() Injector { return &Recorder{} }
