//go:build darwin

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// macOS prints through CUPS: `lpstat` lists printers and their state, `lp`
// queues a job. "-o raw" hands ZPL to a Zebra untouched; a PDF goes through
// the normal filter chain so the driver renders it.
type cupsSpooler struct{}

func newSpooler() Spooler { return cupsSpooler{} }

var (
	lpstatPrinterRe = regexp.MustCompile(`(?m)^printer (\S+) [^\n]*`)
	lpstatDefaultRe = regexp.MustCompile(`(?m)^system default destination: (\S+)`)
	lpRequestRe     = regexp.MustCompile(`request id is (\S+)`)
)

func (cupsSpooler) ListPrinters() ([]Printer, error) {
	out, err := exec.Command("lpstat", "-p", "-d").CombinedOutput()
	if err != nil && !strings.Contains(string(out), "no destinations") {
		// lpstat exits non-zero when there are no printers at all; that is not an error for us.
		if len(out) == 0 {
			return nil, fmt.Errorf("lpstat: %w", err)
		}
	}
	return parseLpstat(string(out)), nil
}

// parseLpstat turns `lpstat -p -d` output into printers. Lines look like
// "printer Zebra_ZD421 is idle.  enabled since ..." or "printer X disabled since ...".
func parseLpstat(out string) []Printer {
	def := ""
	if m := lpstatDefaultRe.FindStringSubmatch(out); m != nil {
		def = m[1]
	}
	var printers []Printer
	for _, m := range lpstatPrinterRe.FindAllStringSubmatch(out, -1) {
		line := strings.ToLower(m[0])
		state := "ready"
		switch {
		case strings.Contains(line, "disabled"), strings.Contains(line, "unable to"), strings.Contains(line, "offline"), strings.Contains(line, "not connected"):
			state = "offline"
		case strings.Contains(line, "is idle"), strings.Contains(line, "now printing"):
			state = "ready"
		default:
			state = "unknown"
		}
		printers = append(printers, Printer{Name: m[1], Default: m[1] == def, State: state})
	}
	return printers
}

func (s cupsSpooler) Print(job PrintJob) (string, error) {
	if !validFormat(job.Format) {
		return "", errBadFormat
	}
	printers, err := s.ListPrinters()
	if err != nil {
		return "", err
	}
	found := false
	for _, p := range printers {
		if p.Name == job.Printer {
			found = true
			if p.State == "offline" {
				return "", errPrinterOffline
			}
		}
	}
	if !found {
		return "", errNoPrinter
	}
	args := []string{"-d", job.Printer, "-t", "USCF label"}
	if job.Format == "ZPL" {
		args = append(args, "-o", "raw")
	}
	cmd := exec.Command("lp", args...)
	cmd.Stdin = bytes.NewReader(job.Data)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("the printer refused the job: %s", msg)
	}
	if m := lpRequestRe.FindStringSubmatch(string(out)); m != nil {
		return m[1], nil
	}
	return "queued", nil
}

// openURL opens the station in the default browser (tray menu).
func openURL(url string) error {
	return exec.Command("open", url).Start()
}

func init() { _ = os.Getenv }
