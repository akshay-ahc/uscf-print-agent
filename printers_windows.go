//go:build windows

package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows prints through the spooler (winspool). A ZPL label is written to the
// printer as a RAW document, which is what a Zebra expects. A PDF label is
// drawn by Windows' own PDF engine and sent through the printer's driver
// (printpdf_windows.ps1), so it prints on any printer with no PDF program.
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
		driver := ""
		if info.DriverName != nil {
			driver = windows.UTF16PtrToString(info.DriverName)
		}
		kind, zpl := classifyPrinter(name, driver)
		out = append(out, Printer{Name: name, Default: name == def, State: state, Driver: driver, Kind: kind, ZPL: zpl})
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
	p, found := kindOf(printers, job.Printer)
	if !found {
		return "", errNoPrinter
	}
	if p.State == "offline" {
		return "", errPrinterOffline
	}
	if job.Format == "ZPL" {
		return printRaw(job.Printer, job.Data)
	}
	if p.Kind == "raw" {
		return "", errRawQueuePDF
	}
	return printPDF(job.Printer, job.Data, p.Kind == "label")
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

//go:embed printpdf_windows.ps1
var printPDFScript []byte

// printPDF prints a PDF label silently with Windows' own PDF engine (no Adobe
// Reader or other PDF program needed): see printpdf_windows.ps1. On a label
// printer it is fitted to a 4x6 label; on a page printer it prints at 4x6 size.
func printPDF(printer string, data []byte, label bool) (string, error) {
	dir := filepath.Join(os.TempDir(), "uscf-print-agent")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	go cleanOldLabels(dir)
	stamp := time.Now().UnixNano()
	pdfPath := filepath.Join(dir, fmt.Sprintf("label-%d.pdf", stamp))
	scriptPath := filepath.Join(dir, "printpdf.ps1")
	if err := os.WriteFile(pdfPath, data, 0o600); err != nil {
		return "", err
	}
	defer os.Remove(pdfPath)
	if err := os.WriteFile(scriptPath, printPDFScript, 0o600); err != nil {
		return "", err
	}
	labelArg := "0"
	if label {
		labelArg = "1"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-File", scriptPath, "-Pdf", pdfPath, "-Printer", printer, "-Label", labelArg)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return "", fmt.Errorf("printing took too long; check that %q is on and has labels", printer)
	}
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if i := strings.Index(msg, "\n"); i > 0 {
			msg = strings.TrimSpace(msg[:i])
		}
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("could not print the PDF label: %s", msg)
	}
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
