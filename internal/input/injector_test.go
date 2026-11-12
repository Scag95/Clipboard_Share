package input

import (
	"sync"
	"testing"
)

// Test that every parsed command dispatches to the matching Injector method
// through Run, using the exported Recorder.
func TestCommandsDispatchToInjector(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"type":"move","dx":4,"dy":-2}`, "move 4 -2"},
		{`{"type":"scroll","dx":1,"dy":0}`, "scroll 1 0"},
		{`{"type":"click","button":"left","mode":"tap"}`, "click left tap"},
		{`{"type":"key","key":"c","mods":["ctrl","shift"]}`, "key c ctrl+shift"},
		{`{"type":"key","key":"enter"}`, "key enter "},
		{`{"type":"text","chars":"hi"}`, `text hi`},
	}
	raw := New()
	rec, ok := raw.(*Recorder)
	if !ok {
		t.Skipf("platform injector %T is not Recorder (windows build)", raw)
	}
	r := rec
	for _, c := range cases {
		cmd, err := Parse([]byte(c.in))
		if err != nil {
			t.Fatalf("Parse(%s): %v", c.in, err)
		}
		if err := cmd.Run(r); err != nil {
			t.Fatalf("Run(%s): %v", c.in, err)
		}
		got := r.Entries()
		last := got[len(got)-1]
		if last != c.want {
			t.Fatalf("Run(%s) recorded %q, want %q", c.in, last, c.want)
		}
	}
}

func TestNewInjectorIsValidInjector(t *testing.T) {
	// Compile-time proof the platform default satisfies the interface.
	var _ Injector = New()
}

func TestRecorderConcurrency(t *testing.T) {
	r := &Recorder{}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = Move{Dx: i, Dy: 0}.Run(r)
		}(i)
	}
	wg.Wait()
	if got := len(r.Entries()); got != 50 {
		t.Fatalf("recorded %d entries, want 50", got)
	}
}

func TestKeyResolutionTable(t *testing.T) {
	// Keys accepted by the parser must resolve to a non-zero virtual key.
	names := []string{"enter", "esc", "tab", "backspace", "delete", "arrowup",
		"arrowdown", "arrowleft", "arrowright", "home", "end", "pageup",
		"pagedown", "space", "c", "z", "9"}
	for _, n := range names {
		if vk := KeyVK(n); vk == 0 {
			t.Errorf("KeyVK(%q) = 0", n)
		}
	}
	if vk := KeyVK("c"); vk != 0x43 { // 'C'
		t.Errorf("KeyVK(c) = %#x, want 0x43", vk)
	}
	if vk := KeyVK("f19"); vk != 0 {
		t.Errorf("KeyVK(f19) = %#x, want 0", vk)
	}
	for _, m := range []string{"ctrl", "alt", "shift", "win"} {
		if ModVK(m) == 0 {
			t.Errorf("ModVK(%q) = 0", m)
		}
	}
	if ModVK("hyper") != 0 {
		t.Error("ModVK(hyper) should be 0")
	}
}

func TestKeyRecordingFormat(t *testing.T) {
	// Canonical Recorder format: "key <name> <mods joined by +>".
	r := &Recorder{}
	cmd, err := Parse([]byte(`{"type":"key","key":"v","mods":["ctrl"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Run(r); err != nil {
		t.Fatal(err)
	}
	if got := r.Entries()[0]; got != "key v ctrl" {
		t.Fatalf("recording = %q, want %q", got, "key v ctrl")
	}
}
