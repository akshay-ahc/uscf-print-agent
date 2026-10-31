package main

import "errors"

// Printer is what the station shows in its sidebar.
type Printer struct {
	Name    string `json:"name"`
	Default bool   `json:"default"`
	// ready | offline | unknown — as far as the OS reports it.
	State string `json:"state"`
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
)

func validFormat(f string) bool { return f == "PDF" || f == "ZPL" }
