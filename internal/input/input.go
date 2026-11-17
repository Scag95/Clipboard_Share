// Package input defines the remote-control event protocol (Spec S6) and the
// platform injector that applies validated events to the OS (Spec S7).
package input

// Injector applies already-validated events to the operating system.
// Implementations: Recorder (tests and non-Windows dev) and Sender (Windows,
// SendInput via purego).
type Injector interface {
	// Move applies a relative pointer delta in pixels.
	Move(dx, dy int) error
	// Scroll applies wheel deltas in notches; positive dy scrolls content down.
	Scroll(dx, dy int) error
	// Click applies a button event: button is "left" or "right", mode is
	// "tap", "down" or "up".
	Click(button, mode string) error
	// Key presses (and releases) a named key with optional modifiers
	// ("ctrl", "alt", "shift", "win").
	Key(key string, mods []string) error
	// Text injects a unicode string as typing.
	Text(chars string) error
}

// Command is a parsed, validated protocol event ready to run on an Injector.
type Command interface {
	Run(Injector) error
}

// Move is a relative pointer movement (S6: {"type":"move"}).
type Move struct{ Dx, Dy int }

func (m Move) Run(i Injector) error { return i.Move(m.Dx, m.Dy) }

// Scroll is a wheel gesture (S6: {"type":"scroll"}).
type Scroll struct{ Dx, Dy int }

func (s Scroll) Run(i Injector) error { return i.Scroll(s.Dx, s.Dy) }

// Click is a button press gesture (S6: {"type":"click"}).
type Click struct{ Button, Mode string }

func (c Click) Run(i Injector) error { return i.Click(c.Button, c.Mode) }

// KeyPress is a named key with modifiers (S6: {"type":"key"}).
type KeyPress struct {
	Key  string
	Mods []string
}

func (k KeyPress) Run(i Injector) error { return i.Key(k.Key, k.Mods) }

// Text is a unicode string typed as a block (S6: {"type":"text"}).
type Text struct{ Chars string }

func (t Text) Run(i Injector) error { return i.Text(t.Chars) }
