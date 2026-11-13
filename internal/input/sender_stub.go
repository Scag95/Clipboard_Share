//go:build !windows && !linux

package input

// New returns the platform injector. Off Windows and Linux (development,
// tests) it records events instead of injecting them (S7).
func New() Injector { return &Recorder{} }
