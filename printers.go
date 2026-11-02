package main

import (
	"errors"
	"regexp"
	"strings"
)

// Printer is what the station shows in its sidebar.
type Printer struct {
	Name    string `json:"name"`
	Default bool   `json:"default"`
	// ready | offline | unknown — as far as the OS reports it.
	State string `json:"state"`
	// Driver (Windows) or make and model (macOS), as the OS reports it.
	Driver string `json:"driver,omitempty"`
	// label = a 4x6 label printer, office = a normal page printer,
	// raw = a queue that only passes bytes through (ZPL only, no PDF).
	Kind string `json:"kind"`
	// ZPL is true for Zebra-type printers: they can take a ZPL label directly
	// (the station's optional fast mode). Every printer except a raw queue prints PDF.
	ZPL bool `json:"zpl"`
}

// Zebra printers that speak ZPL. The LP/TLP 2844 family is Zebra but EPL only.
var zplRe = regexp.MustCompile(`(?i)\b(zebra|zdesigner|zpl)\b|\b(zd|zt|zq|gk|gx|gc|zp|zm|zr|105sl|110xi|220xi|zt)\s?-?\d{3}`)
var eplOnlyRe = regexp.MustCompile(`(?i)\b(t?lp\s?-?2844|lp\s?-?2824|eltron)\b`)

// Brands and words that mean a 4x6 (or other) label printer rather than a page printer.
var labelRe = regexp.MustCompile(`(?i)\b(label|thermal|shipping|4\s?x\s?6|rollo|dymo|munbyn|xprinter|polono|idprt|phomemo|jadens|itari|beeprt|hprt|arkscan|nelko|omezizy|labelife|godex|sato|tsc|datamax|eltron|zebra|zdesigner|citizen\s?cl|honeywell\s?pc\d{2}|brother\s?(ql|td)|ql-\d|td-\d)\b`)
var rawRe = regexp.MustCompile(`(?i)\b(raw|generic\s*/\s*text\s*only|text\s*only)\b`)

// classifyPrinter decides what kind of printer it is from the OS name and driver.
func classifyPrinter(name, driver string) (kind string, zpl bool) {
	// macOS queue names use underscores for spaces ("Zebra_ZD421").
	name, driver = strings.ReplaceAll(name, "_", " "), strings.ReplaceAll(driver, "_", " ")
	text := name + " " + driver
	zpl = zplRe.MatchString(text) && !eplOnlyRe.MatchString(text)
	switch {
	case rawRe.MatchString(driver):
		// A pass-through queue: the label must already be in the printer's language.
		return "raw", zpl || zplRe.MatchString(name)
	case zpl || labelRe.MatchString(text):
		return "label", zpl
	default:
		return "office", false
	}
}

func kindOf(printers []Printer, name string) (Printer, bool) {
	for _, p := range printers {
		if p.Name == name {
			return p, true
		}
	}
	return Printer{}, false
}

// PrintJob is one label to print. ZPL goes to the printer as raw bytes (Zebra);
// PDF goes through the OS so the printer driver renders it.
type PrintJob struct {
	Printer string
	Format  string // "PDF" | "ZPL"
	Data    []byte
}

// Spooler is the OS-specific part: see printers_darwin.go and printers_windows.go.
type Spooler interface {
	ListPrinters() ([]Printer, error)
	Print(job PrintJob) (jobID string, err error)
}

var (
	errNoPrinter      = errors.New("no printer with that name is installed on this computer")
	errPrinterOffline = errors.New("the printer is offline or turned off")
	errBadFormat      = errors.New("format must be PDF or ZPL")
	errRawQueuePDF    = errors.New("this printer is set up without its driver, so it only takes ZPL; turn on fast ZPL mode for it, or add the printer again with its driver")
)

func validFormat(f string) bool { return f == "PDF" || f == "ZPL" }
