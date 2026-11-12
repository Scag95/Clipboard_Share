package input

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// Bounds for protocol fields (S6). Values outside them are rejected without
// ever reaching the injector.
const (
	MaxPointerDelta  = 1000
	MaxScrollNotches = 100
	MaxTextRunes     = 1000
)

// wire mirrors the JSON envelope of S6. Pointers distinguish "absent" from
// zero so we can reject fields that do not belong to the event type.
type wire struct {
	Type   string   `json:"type"`
	Dx     *int     `json:"dx"`
	Dy     *int     `json:"dy"`
	Button *string  `json:"button"`
	Mode   *string  `json:"mode"`
	Key    *string  `json:"key"`
	Mods   []string `json:"mods"`
	Chars  *string  `json:"chars"`
}

// allowedFields per event type (everything else must be absent).
var allowedFields = map[string][]string{
	"move":   {"dx", "dy"},
	"scroll": {"dx", "dy"},
	"click":  {"button", "mode"},
	"key":    {"key", "mods"},
	"text":   {"chars"},
}

var fieldPresent = map[string]func(*wire) bool{
	"dx":     func(w *wire) bool { return w.Dx != nil },
	"dy":     func(w *wire) bool { return w.Dy != nil },
	"button": func(w *wire) bool { return w.Button != nil },
	"mode":   func(w *wire) bool { return w.Mode != nil },
	"key":    func(w *wire) bool { return w.Key != nil },
	"mods":   func(w *wire) bool { return w.Mods != nil },
	"chars":  func(w *wire) bool { return w.Chars != nil },
}

// Parse decodes and validates one protocol event. On error it returns nil
// plus a human-readable reason safe to echo to the client; nothing invalid is
// ever injected (S6).
func Parse(data []byte) (Command, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var w wire
	if err := dec.Decode(&w); err != nil {
		return nil, fmt.Errorf("invalid event: %w", err)
	}
	if err := dec.Decode(&w); !errors.Is(err, io.EOF) {
		return nil, errors.New("expected exactly one event object")
	}

	fields := allowedFields[w.Type]
	if fields == nil {
		return nil, fmt.Errorf("unknown event type %q", w.Type)
	}
	allowed := make(map[string]bool, len(fields))
	for _, f := range fields {
		allowed[f] = true
	}
	for f := range fieldPresent {
		if !allowed[f] && fieldPresent[f](&w) {
			return nil, fmt.Errorf("unexpected field %q for type %q", f, w.Type)
		}
	}

	switch w.Type {
	case "move":
		dx, err := delta("move", "dx", w.Dx, MaxPointerDelta)
		if err != nil {
			return nil, err
		}
		dy, err := delta("move", "dy", w.Dy, MaxPointerDelta)
		if err != nil {
			return nil, err
		}
		return Move{Dx: dx, Dy: dy}, nil
	case "scroll":
		dx, err := delta("scroll", "dx", w.Dx, MaxScrollNotches)
		if err != nil {
			return nil, err
		}
		dy, err := delta("scroll", "dy", w.Dy, MaxScrollNotches)
		if err != nil {
			return nil, err
		}
		return Scroll{Dx: dx, Dy: dy}, nil
	case "click":
		if w.Button == nil {
			return nil, errors.New("click requires field \"button\"")
		}
		if *w.Button != "left" && *w.Button != "right" {
			return nil, fmt.Errorf("click.button %q must be \"left\" or \"right\"", *w.Button)
		}
		if w.Mode == nil {
			return nil, errors.New("click requires field \"mode\"")
		}
		switch *w.Mode {
		case "tap", "down", "up":
		default:
			return nil, fmt.Errorf("click.mode %q must be \"tap\", \"down\" or \"up\"", *w.Mode)
		}
		return Click{Button: *w.Button, Mode: *w.Mode}, nil
	case "key":
		if w.Key == nil || *w.Key == "" {
			return nil, errors.New("key requires a non-empty \"key\"")
		}
		name := strings.ToLower(*w.Key)
		if KeyVK(name) == 0 {
			return nil, fmt.Errorf("unknown key %q", *w.Key)
		}
		mods := make([]string, 0, len(w.Mods))
		for _, m := range w.Mods {
			m = strings.ToLower(m)
			if ModVK(m) == 0 {
				return nil, fmt.Errorf("unknown modifier %q", m)
			}
			mods = append(mods, m)
		}
		return KeyPress{Key: name, Mods: mods}, nil
	case "text":
		if w.Chars == nil {
			return nil, errors.New("text requires field \"chars\"")
		}
		s := *w.Chars
		if s == "" {
			return nil, errors.New("text.chars must not be empty")
		}
		if n := utf8.RuneCountInString(s); n > MaxTextRunes {
			return nil, fmt.Errorf("text.chars too long: %d runes, max %d", n, MaxTextRunes)
		}
		for _, r := range s {
			if r < 0x20 || r == 0x7f {
				return nil, fmt.Errorf("text.chars contains control character %#x", r)
			}
		}
		return Text{Chars: s}, nil
	}
	return nil, fmt.Errorf("unhandled type %q", w.Type) // unreachable
}

func delta(kind, field string, v *int, max int) (int, error) {
	if v == nil {
		return 0, nil
	}
	if *v < -max || *v > max {
		return 0, fmt.Errorf("%s.%s out of range: want [%d,%d], got %d", kind, field, -max, max, *v)
	}
	return *v, nil
}
