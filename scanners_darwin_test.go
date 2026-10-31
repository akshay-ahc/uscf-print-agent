//go:build darwin

package main

import "testing"

func TestParseIoreg(t *testing.T) {
	out := `+-o Root  <class IORegistryEntry>
  +-o AppleT8112USBXHCI@00000000  <class AppleT8112USBXHCI>
    +-o USBScn Module@00100000  <class IOUSBHostDevice>
    |   {
    |     "USB Vendor Name" = "USBScn Chip"
    |     "USB Product Name" = "USBScn Module"
    |     "idVendor" = 536
    |     "idProduct" = 2
    |   }
    +-o USB2.0 Hub@00200000  <class IOUSBHostDevice>
    | |   "USB Product Name" = "USB2.0 Hub"
    | |   "USB Vendor Name" = "Fresco Logic, Inc."
    | |   "idVendor" = 7516
    | |   "idProduct" = 24577
`
	got := parseIoreg(out)
	if len(got) != 1 || got[0].Name != "USBScn Module" || got[0].ID != "0218:0002" {
		t.Fatalf("got %+v", got)
	}
}
