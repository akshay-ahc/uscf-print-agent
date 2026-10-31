// USCF Print Agent: a small program for the shipping PC. It lists the
// computer's printers and prints shipping labels on them, on behalf of the
// Shipping Station page in the browser. Nothing leaves the PC: the page talks
// to it on 127.0.0.1 only, after a one-time pairing with a code from the tray.
//
//	uscf-print-agent               run (tray icon + local API)
//	uscf-print-agent --install     also start at login from now on
//	uscf-print-agent --uninstall   stop starting at login
//	uscf-print-agent --no-tray     run headless (service / debugging)
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

// Version is set by the build (-ldflags "-X main.Version=...").
var Version = "dev"

const stationURL = "https://portal.uscompoundingfactory.com/shipping"

func main() {
	install := flag.Bool("install", false, "start the agent at login from now on, then run")
	uninstall := flag.Bool("uninstall", false, "stop starting at login and exit")
	noTray := flag.Bool("no-tray", false, "run without a tray icon")
	version := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *version {
		fmt.Println("uscf-print-agent", Version)
		return
	}
	if *uninstall {
		if err := removeAutostart(); err != nil {
			log.Fatalf("could not remove the login item: %v", err)
		}
		fmt.Println("USCF Print Agent will no longer start at login.")
		return
	}
	if *install {
		if err := installAutostart(); err != nil {
			log.Fatalf("could not add the login item: %v", err)
		}
		fmt.Println("USCF Print Agent will start at login.")
	}

	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	srv := &Server{cfg: cfg, pairing: NewPairing(), spooler: newSpooler(), version: Version}

	// One agent per computer: if the port is taken, another copy is running.
	ln, err := net.Listen("tcp", srv.addr())
	if err != nil {
		var opErr *net.OpError
		if errors.As(err, &opErr) {
			fmt.Fprintln(os.Stderr, "USCF Print Agent is already running on this computer.")
			os.Exit(0)
		}
		log.Fatalf("listen: %v", err)
	}
	httpSrv := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second}
	go func() {
		log.Printf("USCF Print Agent %s listening on http://%s", Version, srv.addr())
		if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	}()

	if *noTray {
		// Headless: there is no tray to read the pairing code from, so print it.
		if cfg.Token == "" {
			log.Printf("pairing code: %s (type it into the station's printer setup page)", srv.pairing.NewCode())
		}
		select {}
	}
	runTray(srv) // blocks until Quit
	_ = httpSrv.Close()
}
