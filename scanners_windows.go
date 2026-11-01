//go:build windows

package main

import (
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
)

// Windows: ask Plug and Play (SetupAPI) for the present devices directly, with
// no PowerShell, so a listing takes milliseconds and opens no window.
//
// Windows names every keyboard-mode scanner "HID Keyboard Device", so the
// friendly name is useless. The text the scanner itself reports ("USBScn
// Module", "Voyager-1200g") is the USB node's bus-reported description. So:
// collect the vendor:product ids of the HID input devices (keyboards and other
// HID devices), then read the USB node with the same ids for its real name.
func listScanners() ([]Scanner, error) {
	inputs := map[string]bool{}
	err := eachPresentDevice("HID", func(set windows.DevInfo, d *windows.DevInfoData, id string) {
		class := registryString(set, d, windows.SPDRP_CLASS)
		if !strings.EqualFold(class, "Keyboard") && !strings.EqualFold(class, "HIDClass") {
			return
		}
		if vid, pid := parseInstanceID(id); vid != 0 {
			inputs[hex4(vid)+":"+hex4(pid)] = true
		}
	})
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	found := []Scanner{}
	err = eachPresentDevice("USB", func(set windows.DevInfo, d *windows.DevInfoData, id string) {
		vid, pid := parseInstanceID(id)
		key := hex4(vid) + ":" + hex4(pid)
		if vid == 0 || !inputs[key] || seen[key] {
			return // not an input device, or a second interface of one already listed
		}
		product := busReportedName(set, d)
		if product == "" {
			product = registryString(set, d, windows.SPDRP_DEVICEDESC)
		}
		vendor := registryString(set, d, windows.SPDRP_MFG)
		if strings.HasPrefix(vendor, "(") { // "(Standard USB Host Controller)" says nothing
			vendor = ""
		}
		if !isAliasedScanner(vid, pid) && !looksLikeScanner(product, vendor, vid) {
			return
		}
		seen[key] = true
		found = append(found, prettyScanner(product, vendor, vid, pid))
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}

// eachPresentDevice calls fn for every plugged-in device under one enumerator ("USB", "HID").
func eachPresentDevice(enumerator string, fn func(set windows.DevInfo, d *windows.DevInfoData, instanceID string)) error {
	set, err := windows.SetupDiGetClassDevsEx(nil, enumerator, 0, windows.DIGCF_PRESENT|windows.DIGCF_ALLCLASSES, 0, "")
	if err != nil {
		return err
	}
	defer set.Close()
	for i := 0; ; i++ {
		d, err := set.EnumDeviceInfo(i)
		if err == windows.ERROR_NO_MORE_ITEMS {
			return nil
		}
		if err != nil {
			continue
		}
		id, err := set.DeviceInstanceID(d)
		if err != nil {
			continue
		}
		fn(set, d, id)
	}
}

func registryString(set windows.DevInfo, d *windows.DevInfoData, prop windows.SPDRP) string {
	v, err := set.DeviceRegistryProperty(d, prop)
	if err != nil {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// DEVPKEY_Device_BusReportedDeviceDesc: the product string the USB device reports.
var busReportedDeviceDesc = windows.DEVPROPKEY{
	FmtID: windows.DEVPROPGUID{Data1: 0x540b947e, Data2: 0x8b40, Data3: 0x45bc, Data4: [8]byte{0xa8, 0xa2, 0x6a, 0x0b, 0x89, 0x4c, 0xbd, 0xa2}},
	PID:   4,
}

func busReportedName(set windows.DevInfo, d *windows.DevInfoData) string {
	v, err := windows.SetupDiGetDeviceProperty(set, d, &busReportedDeviceDesc)
	if err != nil {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

var instanceIDRe = regexp.MustCompile(`(?i)VID_([0-9A-F]{4})&PID_([0-9A-F]{4})`)

func parseInstanceID(id string) (uint16, uint16) {
	m := instanceIDRe.FindStringSubmatch(id)
	if m == nil {
		return 0, 0
	}
	v, _ := strconv.ParseUint(m[1], 16, 16)
	p, _ := strconv.ParseUint(m[2], 16, 16)
	return uint16(v), uint16(p)
}
