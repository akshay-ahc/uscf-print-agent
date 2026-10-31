//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows prints through the spooler (winspool). ZPL is written to the printer
// as a RAW document, which is what a Zebra expects. A PDF is handed to the
// program Windows uses for PDFs with the "printto" verb (Adobe Reader and most
// viewers support it; Edge does not), so for a Zebra the station sends ZPL.
type winSpooler struct{}

func newSpooler() Spooler { return winSpooler{} }

var (
	winspool              = windows.NewLazySystemDLL("winspool.drv")
	procEnumPrintersW     = winspool.NewProc("EnumPrintersW")
	procGetDefaultPrinter = winspool.NewProc("GetDefaultPrinterW")
	procOpenPrinterW      = winspool.NewProc("OpenPrinterW")
	procClosePrinter      = winspool.NewProc("ClosePrinter")
	procStartDocPrinterW  = winspool.NewProc("StartDocPrinterW")
	procStartPagePrinter  = winspool.NewProc("StartPagePrinter")
	procWritePrinter      = winspool.NewProc("WritePrinter")
	procEndPagePrinter    = winspool.NewProc("EndPagePrinter")
	procEndDocPrinter     = winspool.NewProc("EndDocPrinter")
	shell32               = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteW     = shell32.NewProc("ShellExecuteW")
)

const (
	printerEnumLocal       = 0x00000002
	printerEnumConnections = 0x00000004
	printerStatusOffline   = 0x00000080
	printerStatusError     = 0x00000002
	printerStatusPaperOut  = 0x00000010
	printerStatusNotAvail  = 0x00001000
	printerAttributeWork   = 0x00000400 // PRINTER_ATTRIBUTE_WORK_OFFLINE
)

// PRINTER_INFO_2W, only the fields we read (the rest are kept for layout).
type printerInfo2 struct {
	ServerName         *uint16
	PrinterName        *uint16
	ShareName          *uint16
	PortName           *uint16
	DriverName         *uint16
	Comment            *uint16
	Location           *uint16
	DevMode            uintptr
	SepFile            *uint16
	PrintProcessor     *uint16
	Datatype           *uint16
	Parameters         *uint16
	SecurityDescriptor uintptr
	Attributes         uint32
	Priority           uint32
	DefaultPriority    uint32
	StartTime          uint32
	UntilTime          uint32
	Status             uint32
	Jobs               uint32
	AveragePPM         uint32
}

func (winSpooler) ListPrinters() ([]Printer, error) {
	var needed, returned uint32
	flags := uintptr(printerEnumLocal | printerEnumConnections)
	procEnumPrintersW.Call(flags, 0, 2, 0, 0, uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&returned)))
	if needed == 0 {
		return []Printer{}, nil
	}
	buf := make([]byte, needed)
	r, _, err := procEnumPrintersW.Call(flags, 0, 2, uintptr(unsafe.Pointer(&buf[0])), uintptr(needed), uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&returned)))
	if r == 0 {
		return nil, fmt.Errorf("EnumPrinters: %w", err)
	}
	def := defaultPrinterName()
	infos := unsafe.Slice((*printerInfo2)(unsafe.Pointer(&buf[0])), returned)
	out := make([]Printer, 0, returned)
	for _, info := range infos {
		name := windows.UTF16PtrToString(info.PrinterName)
		state := "ready"
		if info.Status&(printerStatusOffline|printerStatusNotAvail) != 0 || info.Attributes&printerAttributeWork != 0 {
			state = "offline"
		} else if info.Status&(printerStatusError|printerStatusPaperOut) != 0 {
			state = "unknown"
		}
		out = append(out, Printer{Name: name, Default: name == def, State: state})
	}
	return out, nil
}

func defaultPrinterName() string {
	var n uint32
	procGetDefaultPrinter.Call(0, uintptr(unsafe.Pointer(&n)))
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n)
	r, _, _ := procGetDefaultPrinter.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

func (s winSpooler) Print(job PrintJob) (string, error) {
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
	if job.Format == "ZPL" {
		return printRaw(job.Printer, job.Data)
	}
	return printPDF(job.Printer, job.Data)
}

type docInfo1 struct {
	DocName    *uint16
	OutputFile *uint16
	Datatype   *uint16
}

// printRaw spools the bytes untouched (datatype RAW), which is how ZPL reaches a Zebra.
func printRaw(printer string, data []byte) (string, error) {
	name, _ := syscall.UTF16PtrFromString(printer)
	var h windows.Handle
	if r, _, err := procOpenPrinterW.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&h)), 0); r == 0 {
		return "", fmt.Errorf("could not open the printer: %w", err)
	}
	defer procClosePrinter.Call(uintptr(h))
	doc, _ := syscall.UTF16PtrFromString("USCF label")
	raw, _ := syscall.UTF16PtrFromString("RAW")
	di := docInfo1{DocName: doc, Datatype: raw}
	jobID, _, err := procStartDocPrinterW.Call(uintptr(h), 1, uintptr(unsafe.Pointer(&di)))
	if jobID == 0 {
		return "", fmt.Errorf("the printer refused the job: %w", err)
	}
	defer procEndDocPrinter.Call(uintptr(h))
	procStartPagePrinter.Call(uintptr(h))
	defer procEndPagePrinter.Call(uintptr(h))
	var written uint32
	if r, _, err := procWritePrinter.Call(uintptr(h), uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), uintptr(unsafe.Pointer(&written))); r == 0 || int(written) != len(data) {
		return "", fmt.Errorf("could not send the label to the printer: %w", err)
	}
	return fmt.Sprintf("%d", jobID), nil
}

// printPDF saves the PDF and asks the registered PDF program to print it to the named printer.
func printPDF(printer string, data []byte) (string, error) {
	dir := filepath.Join(os.TempDir(), "uscf-print-agent")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("label-%d.pdf", time.Now().UnixNano()))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	verb, _ := syscall.UTF16PtrFromString("printto")
	file, _ := syscall.UTF16PtrFromString(path)
	args, _ := syscall.UTF16PtrFromString(`"` + printer + `"`)
	r, _, _ := procShellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), uintptr(unsafe.Pointer(args)), 0, 0)
	if r <= 32 {
		return "", fmt.Errorf("no PDF program on this computer can print to %q; use ZPL for a Zebra, or install Adobe Reader", printer)
	}
	// The viewer reads the file asynchronously; clean up old files instead of this one.
	go cleanOldLabels(dir)
	return "pdf", nil
}

func cleanOldLabels(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-time.Hour)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

func openURL(url string) error {
	verb, _ := syscall.UTF16PtrFromString("open")
	target, _ := syscall.UTF16PtrFromString(url)
	r, _, err := procShellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(target)), 0, 0, 1)
	if r <= 32 {
		return err
	}
	return nil
}
