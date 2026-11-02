//go:build darwin

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
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
	printers := parseLpstat(string(out))
	for i := range printers {
		printers[i].Driver = makeAndModel(printers[i].Name)
		printers[i].Kind, printers[i].ZPL = classifyPrinter(printers[i].Name, printers[i].Driver)
	}
	return printers, nil
}

var makeModelRe = regexp.MustCompile(`printer-make-and-model='([^']*)'|printer-make-and-model=(\S+)`)

// The make and model barely ever changes, and the station asks for status
// every few seconds, so each printer's answer is kept for a minute.
var (
	makeModelMu    sync.Mutex
	makeModelCache = map[string]struct {
		value string
		at    time.Time
	}{}
)

// makeAndModel asks CUPS for the printer's make and model ("Zebra ZP 450-200",
// "HP LaserJet ...", or "Local Raw Printer" for a pass-through queue).
func makeAndModel(name string) string {
	makeModelMu.Lock()
	if c, ok := makeModelCache[name]; ok && time.Since(c.at) < time.Minute {
		makeModelMu.Unlock()
		return c.value
	}
	makeModelMu.Unlock()
	out, _ := exec.Command("lpoptions", "-p", name).Output()
	value := parseMakeAndModel(string(out))
	makeModelMu.Lock()
	makeModelCache[name] = struct {
		value string
		at    time.Time
	}{value, time.Now()}
	makeModelMu.Unlock()
	return value
}

func parseMakeAndModel(out string) string {
	m := makeModelRe.FindStringSubmatch(out)
	if m == nil {
		return ""
	}
	if m[1] != "" {
		return m[1]
	}
	return m[2]
}

// lpArgs builds the lp command for a job. A PDF on a label printer is fitted
// to a 4x6 label; on a page printer it prints at its own 4x6 size. ZPL goes raw.
func lpArgs(job PrintJob, p Printer) []string {
	args := []string{"-d", job.Printer, "-t", "USCF label"}
	switch {
	case job.Format == "ZPL":
		args = append(args, "-o", "raw")
	case p.Kind == "label":
		args = append(args, "-o", "media=w288h432", "-o", "fit-to-page")
	}
	return args
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
	p, found := kindOf(printers, job.Printer)
	if !found {
		return "", errNoPrinter
	}
	if p.State == "offline" {
		return "", errPrinterOffline
	}
	if job.Format == "PDF" && p.Kind == "raw" {
		return "", errRawQueuePDF
	}
	cmd := exec.Command("lp", lpArgs(job, p)...)
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
