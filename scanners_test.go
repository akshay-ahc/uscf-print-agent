package main

import (
	"testing"
	"time"
)

func TestLooksLikeScanner(t *testing.T) {
	cases := []struct {
		product, vendor string
		vid             uint16
		want            bool
	}{
		{"Voyager-1200g", "Honeywell Imaging & Mobility", 0x0c2e, true},
		{"Symbol Bar Code Scanner", "Symbol Technologies, Inc", 0x05e0, true},
		{"USB Keyboard", "Unknown", 0x1a86, false}, // budget chip, no scanner word
		{"NADAMOO Wireless Barcode Scanner", "", 0x1a86, true},
		{"Apple Internal Keyboard / Trackpad", "Apple Inc.", 0x05ac, false},
		{"Zebra ZD421", "Zebra Technologies", 0x0a5f, false}, // a label printer, not a scanner
		{"USB Receiver", "Logitech", 0x046d, false},
		{"Gryphon GD4430", "Datalogic", 0x05f9, true},
		{"USBScn Module", "USBScn Chip", 0x0218, true}, // generic scanner chip, as seen on the test Mac
	}
	for _, c := range cases {
		if got := looksLikeScanner(c.product, c.vendor, c.vid); got != c.want {
			t.Errorf("%q / %q / %04x: got %v want %v", c.product, c.vendor, c.vid, got, c.want)
		}
	}
	if a := prettyScanner("USBScn Module", "USBScn Chip", 0x0218, 0x0210); a.Name != "HT410" || a.Vendor != "Helett" {
		t.Errorf("alias: %+v", a)
	}
	s := prettyScanner("Voyager-1200g", "Honeywell Imaging & Mobility Technologies", 0x0c2e, 0x0b61)
	if s.Name != "Voyager-1200g" || s.Vendor != "Honeywell" || s.ID != "0c2e:0b61" {
		t.Errorf("pretty: %+v", s)
	}
	// Windows shows the Helett as "HID Keyboard Device"; its ids alone must be enough.
	if !isAliasedScanner(0x0218, 0x0210) || isAliasedScanner(0x046d, 0xc548) {
		t.Errorf("isAliasedScanner")
	}
}

func TestCachedScannersDoesNotWaitOnRefresh(t *testing.T) {
	calls := 0
	release := make(chan struct{})
	list := func() ([]Scanner, error) {
		calls++
		if calls > 1 {
			<-release // a slow OS listing
		}
		return []Scanner{{Name: "HT410"}}, nil
	}
	get := cachedScanners(list)
	if s, _ := get(); len(s) != 1 {
		t.Fatalf("first call: %v", s)
	}
	// Let the cache go stale; the next call must return the old list at once.
	time.Sleep(5100 * time.Millisecond)
	done := make(chan struct{})
	go func() { get(); close(done) }()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("status waited for the scanner refresh")
	}
	close(release)
}
