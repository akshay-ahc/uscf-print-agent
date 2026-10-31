//go:build darwin

package main

import "testing"

func TestParseLpstat(t *testing.T) {
	out := `printer Zebra_ZD421 is idle.  enabled since Fri Oct 31 10:02:11 2026
printer HP_OfficeJet disabled since Fri Oct 31 09:00:00 2026 -
	Unable to connect to printer
printer Brother_HL now printing Brother_HL-12.  enabled since Fri Oct 31 10:05:00 2026
system default destination: Zebra_ZD421
`
	got := parseLpstat(out)
	want := []Printer{
		{Name: "Zebra_ZD421", Default: true, State: "ready"},
		{Name: "HP_OfficeJet", Default: false, State: "offline"},
		{Name: "Brother_HL", Default: false, State: "ready"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("printer %d: got %+v want %+v", i, got[i], want[i])
		}
	}
	if len(parseLpstat("lpstat: No destinations added.\n")) != 0 {
		t.Fatal("no printers should parse as empty")
	}
}
