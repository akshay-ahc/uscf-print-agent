package main

import (
	"regexp"
	"strings"
)

// Scanner is a USB barcode scanner the OS reports as plugged in. Most scanners
// act as a keyboard, so the browser cannot tell they exist; the agent asks the
// OS for the USB device list and picks out the ones that look like scanners.
type Scanner struct {
	Name   string `json:"name"`
	Vendor string `json:"vendor"`
	// USB vendor:product in hex, e.g. "0c2e:0b61", when the OS gives it.
	ID string `json:"id,omitempty"`
}

// USB vendor ids of the common barcode scanner makers.
var scannerVendors = map[uint16]string{
	0x0c2e: "Honeywell",
	0x05e0: "Zebra / Symbol",
	0x05f9: "Datalogic",
	0x0536: "Socket Mobile",
	0x1eab: "Newland / Fujian",
	0x1a86: "", // QinHeng chips used by many budget scanners; name decides
	0x2dd6: "Tera",
	0x0483: "", // STMicro chips, also used in scanners; name decides
	0x04b4: "", // Cypress; name decides
	0x1fc9: "", // NXP; name decides
	0x0218: "", // "USBScn" generic scanner module; the name decides
	0x0801: "Mag-Tek",
	0x0c27: "Code Corporation",
	0x1d6b: "", // Linux root hubs; never a scanner
	0x065a: "Optoelectronics",
	0x04e8: "", // Samsung
	0x0b00: "Hand Held Products",
	0x1659: "Prolific",
}

var scannerWords = regexp.MustCompile(`(?i)\b(scanner|bar ?code|barcode|imager|usbscn|scn module|honeywell|voyager|xenon|granit|hyperion|symbol|zebra|datalogic|gryphon|quickscan|socket ?scan|inateck|nadamoo|eyoyo|tera|netum|wasp|opticon|code reader|cr\d{4}|ds\d{4}|li\d{4}|ls\d{4})\b`)

// looksLikeScanner decides from the USB product/vendor strings and ids.
func looksLikeScanner(product, vendor string, vendorID uint16) bool {
	text := product + " " + vendor
	if scannerWords.MatchString(text) {
		return true
	}
	if name, known := scannerVendors[vendorID]; known && name != "" {
		// A known scanner maker, but only when the device is an input device
		// (their printers and radios share the vendor id): the callers only pass
		// HID/keyboard-class devices, so this is enough.
		return true
	}
	return false
}

// prettyScanner tidies the OS strings ("Honeywell Imaging & Mobility" + "Voyager-1200g").
func prettyScanner(product, vendor string, vendorID uint16, productID uint16) Scanner {
	vendor = strings.TrimSpace(vendor)
	if known := scannerVendors[vendorID]; known != "" && (vendor == "" || len(vendor) > 24) {
		vendor = known
	}
	product = strings.TrimSpace(product)
	if product == "" {
		product = "Barcode scanner"
	}
	id := ""
	if vendorID != 0 {
		id = strings.ToLower(strings.TrimSpace(hex4(vendorID) + ":" + hex4(productID)))
	}
	return Scanner{Name: product, Vendor: vendor, ID: id}
}

func hex4(v uint16) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[v>>12&0xf], digits[v>>8&0xf], digits[v>>4&0xf], digits[v&0xf]})
}
