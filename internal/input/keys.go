package input

import "strings"

// Windows virtual-key codes for the named keys accepted by the protocol (S6).
var namedKeys = map[string]uint16{
	"enter":      0x0D,
	"esc":        0x1B,
	"tab":        0x09,
	"backspace":  0x08,
	"delete":     0x2E,
	"insert":     0x2D,
	"arrowup":    0x26,
	"arrowdown":  0x28,
	"arrowleft":  0x25,
	"arrowright": 0x27,
	"home":       0x24,
	"end":        0x23,
	"pageup":     0x21,
	"pagedown":   0x22,
	"space":      0x20,
	"f1":         0x70,
	"f2":         0x71,
	"f3":         0x72,
	"f4":         0x73,
	"f5":         0x74,
	"f6":         0x75,
	"f7":         0x76,
	"f8":         0x77,
	"f9":         0x78,
	"f10":        0x79,
	"f11":        0x7A,
	"f12":        0x7B,
}

// Keys that require KEYEVENTF_EXTENDEDKEY on Windows.
var extendedKeys = map[string]bool{
	"delete": true, "insert": true, "arrowup": true, "arrowdown": true,
	"arrowleft": true, "arrowright": true, "home": true, "end": true,
	"pageup": true, "pagedown": true,
}

// Modifier virtual-key codes.
var modifierKeys = map[string]uint16{
	"ctrl":  0x11, // VK_CONTROL
	"alt":   0x12, // VK_MENU
	"shift": 0x10, // VK_SHIFT
	"win":   0x5B, // VK_LWIN (extended)
}

// KeyVK returns the Windows virtual-key code for an accepted key name
// (lowercase), or 0 if the name is not valid. Single alphanumeric characters
// map to their VK letter/digit codes.
func KeyVK(name string) uint16 {
	name = strings.ToLower(name)
	if vk, ok := namedKeys[name]; ok {
		return vk
	}
	if len(name) != 1 {
		return 0
	}
	c := name[0]
	switch {
	case c >= 'a' && c <= 'z':
		return uint16(c - 32) // VK letters match uppercase ASCII
	case c >= '0' && c <= '9':
		return uint16(c)
	}
	return 0
}

// IsExtended reports whether the key needs KEYEVENTF_EXTENDEDKEY.
func IsExtended(name string) bool {
	return extendedKeys[strings.ToLower(name)]
}

// ModVK returns the virtual-key code for a modifier name, or 0 if unknown.
func ModVK(name string) uint16 {
	return modifierKeys[strings.ToLower(name)]
}

// ModifierOrder returns the mods to press (in order) and release (reverse).
func normalizeMods(mods []string) []uint16 {
	out := make([]uint16, 0, len(mods))
	for _, m := range mods {
		if vk := ModVK(m); vk != 0 {
			out = append(out, vk)
		}
	}
	return out
}
