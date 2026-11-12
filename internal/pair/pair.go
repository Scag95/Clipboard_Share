// Package pair implements PIN-based pairing: the server owns a short 6-digit
// PIN and hands out opaque session tokens to devices that present it.
package pair

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"
)

// MaxFailures is the number of consecutive wrong-PIN attempts allowed before
// the PIN is rotated (spent) and a fresh one is generated.
const MaxFailures = 5

// MaxRotations caps how many times the PIN may be spent by failed attempts
// during one server run. Each rotation is a fresh 6-digit target, so without
// a lifetime cap an unbounded request rate makes online brute force
// statistically trivial (~1e6 guesses per PIN). Exhausting this cap locks
// pairing until the server restarts, bounding total guesses per run at
// MaxFailures*MaxRotations = 25.
const MaxRotations = 5

// Broker holds the current PIN and the set of issued session tokens. It is
// safe for concurrent use.
type Broker struct {
	mu         sync.Mutex
	pin        string
	tokens     map[string]struct{}
	failures   int
	rotations  int
	onRotation func(string)
}

// NewBroker returns a broker with a fresh random 6-digit PIN.
func NewBroker() *Broker {
	return &Broker{pin: randomPIN(), tokens: make(map[string]struct{})}
}

// OnPINChange registers a callback invoked (synchronously, outside the
// broker's lock) with the new PIN whenever the PIN is rotated.
func (b *Broker) OnPINChange(fn func(string)) {
	b.mu.Lock()
	b.onRotation = fn
	b.mu.Unlock()
}

// Pin returns the currently accepted PIN.
func (b *Broker) Pin() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.pin
}

// Pair exchanges a PIN for a new session token. A correct PIN always yields a
// fresh token; a wrong PIN yields ("", false) and, after MaxFailures
// consecutive misses, rotates the PIN out.
func (b *Broker) Pair(pin string) (string, bool) {
	b.mu.Lock()
	if b.rotations >= MaxRotations {
		// Pairing is spent for this server run; even the correct PIN is
		// refused until restart. See MaxRotations.
		b.mu.Unlock()
		return "", false
	}
	var cb func(string)
	if subtle.ConstantTimeCompare([]byte(pin), []byte(b.pin)) != 1 {
		b.failures++
		rotated := false
		if b.failures >= MaxFailures {
			b.pin = randomPIN()
			b.failures = 0
			b.rotations++
			rotated = true
			cb = b.onRotation
		}
		newPin := b.pin
		b.mu.Unlock()
		if rotated && cb != nil {
			cb(newPin)
		}
		return "", false
	}
	b.failures = 0
	token := randomToken()
	b.tokens[token] = struct{}{}
	b.mu.Unlock()
	return token, true
}

// Locked reports whether pairing has exhausted its lifetime rotation cap and
// is disabled until the server restarts.
func (b *Broker) Locked() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.rotations >= MaxRotations
}

// Validate reports whether token was issued by this broker and is still
// active.
func (b *Broker) Validate(token string) bool {
	if token == "" {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for t := range b.tokens {
		if subtle.ConstantTimeCompare([]byte(t), []byte(token)) == 1 {
			return true
		}
	}
	return false
}

// Revoke removes a token (used when a session is disconnected for good).
func (b *Broker) Revoke(token string) {
	b.mu.Lock()
	delete(b.tokens, token)
	b.mu.Unlock()
}

func randomPIN() string {
	// Rejection sampling keeps the PIN uniform over 000000..999999.
	const limit = ^uint32(0) - (^uint32(0) % 1_000_000)
	var buf [4]byte
	for {
		if _, err := rand.Read(buf[:]); err != nil {
			panic("pair: crypto/rand unavailable: " + err.Error())
		}
		n := binary.BigEndian.Uint32(buf[:])
		if n < limit {
			return fmt.Sprintf("%06d", n%1_000_000)
		}
	}
}

func randomToken() string {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic("pair: crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(buf[:])
}
