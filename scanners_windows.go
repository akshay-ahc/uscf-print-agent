//go:build windows

package main

import (
	"encoding/json"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

// Windows: ask Plug and Play for the HID devices (a keyboard-mode scanner is
// one) with their friendly names and hardware ids (VID_xxxx&PID_xxxx).
func listScanners() ([]Scanner, error) {
	ps := `Get-PnpDevice -PresentOnly | Where-Object { $_.Class -in @('HIDClass','Keyboard','USB','Ports') } | Select-Object FriendlyName, Manufacturer, InstanceId | ConvertTo-Json -Compress`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	out = []byte(strings.TrimSpace(string(out)))
	if len(out) == 0 {
		return []Scanner{}, nil
	}
	type dev struct {
		FriendlyName string `json:"FriendlyName"`
		Manufacturer string `json:"Manufacturer"`
		InstanceId   string `json:"InstanceId"`
	}
	var devs []dev
	if out[0] == '{' { // a single object when only one device matched
		var d dev
		if err := json.Unmarshal(out, &d); err != nil {
			return nil, err
		}
		devs = []dev{d}
	} else if err := json.Unmarshal(out, &devs); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	found := []Scanner{}
	for _, d := range devs {
		vid, pid := parseInstanceID(d.InstanceId)
		if !looksLikeScanner(d.FriendlyName, d.Manufacturer, vid) {
			continue
		}
		s := prettyScanner(d.FriendlyName, d.Manufacturer, vid, pid)
		key := s.ID + "|" + s.Name
		if seen[key] { // the same scanner shows up as several HID interfaces
			continue
		}
		seen[key] = true
		found = append(found, s)
	}
	return found, nil
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
