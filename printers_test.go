package main

import "testing"

func TestClassifyPrinter(t *testing.T) {
	cases := []struct {
		name, driver string
		kind         string
		zpl          bool
	}{
		{"Zebra ZP 450-200", "ZDesigner ZP 450-200", "label", true},
		{"ZD421", "ZDesigner ZD421-203dpi ZPL", "label", true},
		{"Zebra_ZD420", "Zebra ZD420", "label", true},
		{"Zebra LP2844", "ZDesigner LP 2844", "label", false}, // EPL only
		{"Rollo Printer", "Rollo Printer", "label", false},
		{"DYMO LabelWriter 4XL", "DYMO LabelWriter 4XL", "label", false},
		{"MUNBYN ITPP941", "MUNBYN Thermal Printer", "label", false},
		{"Brother QL-1110NWB", "Brother QL-1110NWB", "label", false},
		{"HP LaserJet Pro M404", "HP LaserJet Pro M404-M405 PCL-6 (V4)", "office", false},
		{"Canon_TS3300", "Canon TS3300 series", "office", false},
		{"Microsoft Print to PDF", "Microsoft Print To PDF", "office", false},
		{"Zebra_raw", "Local Raw Printer", "raw", true},
		{"Shipping", "Generic / Text Only", "raw", false},
	}
	for _, c := range cases {
		kind, zpl := classifyPrinter(c.name, c.driver)
		if kind != c.kind || zpl != c.zpl {
			t.Errorf("%q / %q: got %s zpl=%v, want %s zpl=%v", c.name, c.driver, kind, zpl, c.kind, c.zpl)
		}
	}
}
