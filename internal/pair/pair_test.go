package pair

import "testing"

func TestPairCorrectIssuesWorkingToken(t *testing.T) {
	b := NewBroker()
	tok, ok := b.Pair(b.Pin())
	if !ok {
		t.Fatalf("Pair(%q) with the current PIN failed", b.Pin())
	}
	if tok == "" {
		t.Fatal("Pair returned an empty token")
	}
	if !b.Validate(tok) {
		t.Fatal("issued token does not validate")
	}
}

func TestPairWrongPinRejected(t *testing.T) {
	b := NewBroker()
	tok, ok := b.Pair("000000")
	if b.Pin() == "000000" { // avoid flaky collision on the random PIN
		t.Skip("generated PIN is 000000")
	}
	if ok || tok != "" {
		t.Fatalf("wrong PIN accepted: ok=%v token=%q", ok, tok)
	}
	if b.Validate(tok) {
		t.Fatal("empty/failed token validates")
	}
}

func TestValidateUnknownTokenRejected(t *testing.T) {
	b := NewBroker()
	if _, ok := b.Pair(b.Pin()); !ok {
		t.Fatal("setup: pairing failed")
	}
	for _, tok := range []string{"", "nope", "deadbeef"} {
		if b.Validate(tok) {
			t.Fatalf("unknown token %q accepted", tok)
		}
	}
}

func TestPinPairsMultipleDevices(t *testing.T) {
	b := NewBroker()
	pin := b.Pin()
	t1, ok1 := b.Pair(pin)
	t2, ok2 := b.Pair(pin)
	if !ok1 || !ok2 {
		t.Fatalf("second pairing with same PIN rejected: %v %v", ok1, ok2)
	}
	if t1 == t2 {
		t.Fatal("two pairings issued the same token")
	}
	if !b.Validate(t1) || !b.Validate(t2) {
		t.Fatal("both tokens must validate")
	}
}

func TestTooManyFailuresRotatesPin(t *testing.T) {
	b := NewBroker()
	old := b.Pin()
	for i := 0; i < MaxFailures; i++ {
		if _, ok := b.Pair("111111"); ok && old != "111111" {
			t.Fatalf("attempt %d wrongly accepted", i)
		}
	}
	if b.Pin() == old {
		t.Fatal("PIN was not rotated after MaxFailures wrong attempts")
	}
	if _, ok := b.Pair(old); ok {
		t.Fatal("spent (rotated-out) PIN still pairs")
	}
	if _, ok := b.Pair(b.Pin()); !ok {
		t.Fatal("fresh PIN does not pair")
	}
}

func TestRotationCallbackFires(t *testing.T) {
	b := NewBroker()
	old := b.Pin()
	seen := make(chan string, 1)
	b.OnPINChange(func(p string) { seen <- p })
	for i := 0; i < MaxFailures; i++ {
		b.Pair("222222")
	}
	select {
	case p := <-seen:
		if p == old || p == "" {
			t.Fatalf("callback got bad PIN %q (old was %q)", p, old)
		}
	default:
		t.Fatal("OnPINChange callback did not fire on rotation")
	}
}

func TestPinIsSixDigits(t *testing.T) {
	for i := 0; i < 200; i++ {
		p := NewBroker().Pin()
		if len(p) != 6 {
			t.Fatalf("PIN %q is not 6 digits", p)
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				t.Fatalf("PIN %q has non-digit %q", p, c)
			}
		}
	}
}
