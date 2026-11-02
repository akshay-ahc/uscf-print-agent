//go:build darwin

package main

import (
	"strings"
	"testing"
)

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

func TestMakeAndModelAndLpArgs(t *testing.T) {
	out := "copies=1 device-uri=usb://Zebra/ZP%20450 printer-make-and-model='Zebra ZP 450-200' printer-type=4\n"
	if got := parseMakeAndModel(out); got != "Zebra ZP 450-200" {
		t.Errorf("make and model: %q", got)
	}
	if got := parseMakeAndModel("printer-make-and-model=Local\n"); got != "Local" {
		t.Errorf("unquoted: %q", got)
	}
	label := Printer{Name: "Rollo", Kind: "label"}
	if got := strings.Join(lpArgs(PrintJob{Printer: "Rollo", Format: "PDF"}, label), " "); !strings.Contains(got, "media=w288h432") || !strings.Contains(got, "fit-to-page") {
		t.Errorf("label PDF args: %s", got)
	}
	office := Printer{Name: "HP", Kind: "office"}
	if got := strings.Join(lpArgs(PrintJob{Printer: "HP", Format: "PDF"}, office), " "); strings.Contains(got, "media=") || strings.Contains(got, "raw") {
		t.Errorf("office PDF args: %s", got)
	}
	if got := strings.Join(lpArgs(PrintJob{Printer: "Zebra", Format: "ZPL"}, Printer{Kind: "label", ZPL: true}), " "); !strings.Contains(got, "-o raw") {
		t.Errorf("ZPL args: %s", got)
	}
}
