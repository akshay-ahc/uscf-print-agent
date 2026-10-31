//go:build darwin

package main

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// macOS: `ioreg -p IOUSB -l` prints every USB device with its product and
// vendor names and numeric ids. (system_profiler can print nothing when the
// agent runs from a launch agent, so the I/O registry is read directly.)
func listScanners() ([]Scanner, error) {
	out, err := exec.Command("ioreg", "-p", "IOUSB", "-l", "-w", "0").Output()
	if err != nil {
		return nil, err
	}
	return parseIoreg(string(out)), nil
}

var (
	ioregDeviceRe = regexp.MustCompile(`^\s*[|\s]*\+-o `)
	ioregFieldRe  = regexp.MustCompile(`"(USB Product Name|USB Vendor Name|idVendor|idProduct)" = (?:"([^"]*)"|(\d+))`)
)

// parseIoreg groups the registry's lines per device ("+-o" starts one) and
// keeps the devices that look like barcode scanners.
func parseIoreg(out string) []Scanner {
	type dev struct {
		product, vendor string
		vid, pid        uint16
	}
	var devices []dev
	var cur *dev
	for _, line := range strings.Split(out, "\n") {
		if ioregDeviceRe.MatchString(line) {
			devices = append(devices, dev{})
			cur = &devices[len(devices)-1]
			continue
		}
		if cur == nil {
			continue
		}
		m := ioregFieldRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		switch m[1] {
		case "USB Product Name":
			cur.product = m[2]
		case "USB Vendor Name":
			cur.vendor = m[2]
		case "idVendor":
			if v, err := strconv.ParseUint(m[3], 10, 16); err == nil {
				cur.vid = uint16(v)
			}
		case "idProduct":
			if v, err := strconv.ParseUint(m[3], 10, 16); err == nil {
				cur.pid = uint16(v)
			}
		}
	}
	found := []Scanner{}
	for _, d := range devices {
		if d.product == "" && d.vendor == "" {
			continue
		}
		if looksLikeScanner(d.product, d.vendor, d.vid) {
			found = append(found, prettyScanner(d.product, d.vendor, d.vid, d.pid))
		}
	}
	return found
}
