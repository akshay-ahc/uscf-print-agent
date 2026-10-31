package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// Pairing ties one browser (the station page on this PC) to this agent. The
// tray icon shows a 6-digit code; the page sends it once and gets a token it
// then presents on every print. The code is single-use and short-lived; a new
// one can be made from the tray at any time (which also forgets the old token).
type Pairing struct {
	mu      sync.Mutex
	code    string
	expires time.Time
	now     func() time.Time
}

const pairingCodeTTL = 10 * time.Minute

func NewPairing() *Pairing { return &Pairing{now: time.Now} }

// NewCode issues a fresh 6-digit code, replacing any earlier one.
func (p *Pairing) NewCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		panic(err) // crypto/rand failing means the machine is unusable
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.code = fmt.Sprintf("%06d", n.Int64())
	p.expires = p.now().Add(pairingCodeTTL)
	return p.code
}

// Code returns the current code and whether it is still valid.
func (p *Pairing) Code() (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.code, p.code != "" && p.now().Before(p.expires)
}

// Redeem checks a code typed into the station page. On success the code is
// consumed and a new random token is returned for the caller to store.
func (p *Pairing) Redeem(code string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.code == "" || !p.now().Before(p.expires) {
		return "", false
	}
	if subtle.ConstantTimeCompare([]byte(code), []byte(p.code)) != 1 {
		return "", false
	}
	p.code = ""
	return newToken(), true
}

func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// tokenMatches compares the bearer token from a request with the stored one.
func tokenMatches(stored, given string) bool {
	return stored != "" && subtle.ConstantTimeCompare([]byte(stored), []byte(given)) == 1
}
