package input

import (
	"strings"
	"testing"
)

func mustParse(t *testing.T, s string) Command {
	t.Helper()
	c, err := Parse([]byte(s))
	if err != nil {
		t.Fatalf("Parse(%s) = err %v, want success", s, err)
	}
	return c
}

func mustFail(t *testing.T, s string) string {
	t.Helper()
	c, err := Parse([]byte(s))
	if err == nil {
		t.Fatalf("Parse(%s) = %+v, want error", s, c)
	}
	if c != nil {
		t.Fatalf("Parse(%s) returned command alongside error", s)
	}
	return err.Error()
}

func TestParseMove(t *testing.T) {
	c := mustParse(t, `{"type":"move","dx":10,"dy":-3}`)
	m, ok := c.(Move)
	if !ok {
		t.Fatalf("type = %T, want Move", c)
	}
	if m.Dx != 10 || m.Dy != -3 {
		t.Fatalf("Move = %+v", m)
	}
	// Missing deltas default to zero.
	m2 := mustParse(t, `{"type":"move","dy":5}`).(Move)
	if m2.Dx != 0 || m2.Dy != 5 {
		t.Fatalf("Move defaults = %+v", m2)
	}
}

func TestParseMoveOutOfRange(t *testing.T) {
	msg := mustFail(t, `{"type":"move","dx":1001,"dy":0}`)
	if !strings.Contains(msg, "dx") {
		t.Fatalf("error %q should name the offending field", msg)
	}
	mustFail(t, `{"type":"move","dx":-1001}`)
	mustParse(t, `{"type":"move","dx":1000,"dy":-1000}`) // boundary accepted
}

func TestParseNonNumericDelta(t *testing.T) {
	mustFail(t, `{"type":"move","dx":"abc"}`)
	mustFail(t, `{"type":"move","dx":1.5}`)
}

func TestParseUnknownType(t *testing.T) {
	mustFail(t, `{"type":"dance"}`)
	mustFail(t, `{"dx":1}`)
}

func TestParseUnknownFieldRejected(t *testing.T) {
	mustFail(t, `{"type":"move","dx":1,"dyy":2}`) // typo
	mustFail(t, `{"type":"text","chars":"hi","dx":1}`)
}

func TestParseMalformedJSON(t *testing.T) {
	mustFail(t, `{`)
	mustFail(t, `[]`)
}

func TestParseClick(t *testing.T) {
	for _, s := range []string{
		`{"type":"click","button":"left","mode":"tap"}`,
		`{"type":"click","button":"right","mode":"down"}`,
		`{"type":"click","button":"right","mode":"up"}`,
	} {
		c := mustParse(t, s)
		if _, ok := c.(Click); !ok {
			t.Fatalf("type = %T, want Click", c)
		}
	}
	mustFail(t, `{"type":"click","button":"middle","mode":"tap"}`)
	mustFail(t, `{"type":"click","button":"left","mode":"double"}`)
	mustFail(t, `{"type":"click","button":"left"}`) // mode required
	mustFail(t, `{"type":"click","mode":"tap"}`)    // button required
}

func TestParseScroll(t *testing.T) {
	s := mustParse(t, `{"type":"scroll","dx":0,"dy":3}`).(Scroll)
	if s.Dx != 0 || s.Dy != 3 {
		t.Fatalf("Scroll = %+v", s)
	}
	mustFail(t, `{"type":"scroll","dy":101}`)
	mustParse(t, `{"type":"scroll","dy":100}`)
}

func TestParseKey(t *testing.T) {
	k := mustParse(t, `{"type":"key","key":"c","mods":["ctrl"]}`).(KeyPress)
	if k.Key != "c" || len(k.Mods) != 1 || k.Mods[0] != "ctrl" {
		t.Fatalf("KeyPress = %+v", k)
	}
	// Named keys from the spec: enter, esc, arrows, plus common editing keys.
	for _, name := range []string{"enter", "esc", "tab", "backspace", "delete",
		"arrowup", "arrowdown", "arrowleft", "arrowright", "home", "end",
		"pageup", "pagedown", "space", "a", "z", "0", "9"} {
		if _, err := Parse([]byte(`{"type":"key","key":"` + name + `"}`)); err != nil {
			t.Fatalf("valid key %q rejected: %v", name, err)
		}
	}
	mustFail(t, `{"type":"key","key":"f19"}`)
	mustFail(t, `{"type":"key","key":"ctrl"}`) // modifiers are mods, not keys
	mustFail(t, `{"type":"key","key":"c","mods":["hyper"]}`)
	mustFail(t, `{"type":"key","key":""}`)
}

func TestParseText(t *testing.T) {
	tx := mustParse(t, `{"type":"text","chars":"hola ñ 123"}`).(Text)
	if tx.Chars != "hola ñ 123" {
		t.Fatalf("Text = %+v", tx)
	}
	mustFail(t, `{"type":"text","chars":""}`)
	mustFail(t, `{"type":"text","chars":"a\tb"}`) // no control chars
	mustFail(t, `{"type":"text"}`)
	long := `{"type":"text","chars":"` + strings.Repeat("x", MaxTextRunes+1) + `"}`
	mustFail(t, long)
	mustParse(t, `{"type":"text","chars":"`+strings.Repeat("x", MaxTextRunes)+`"}`)
}

func TestLimitsSane(t *testing.T) {
	if MaxPointerDelta <= 0 || MaxScrollNotches <= 0 || MaxTextRunes <= 0 {
		t.Fatal("limits must be positive")
	}
}
