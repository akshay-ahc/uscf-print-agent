package main

import (
	"fmt"
	"log"

	"fyne.io/systray"
)

// The tray (menu bar on macOS) is the only screen the agent has. It shows
// whether this PC is paired with the station, the pairing code when needed,
// and the printers the OS reports; the station page does the rest.
func runTray(srv *Server) {
	systray.Run(func() { trayReady(srv) }, func() {})
}

func trayReady(srv *Server) {
	systray.SetTemplateIcon(trayIcon, trayIcon)
	systray.SetTooltip("USCF Print Agent")

	status := systray.AddMenuItem("USCF Print Agent "+Version, "")
	status.Disable()
	paired := systray.AddMenuItem("", "")
	paired.Disable()
	codeItem := systray.AddMenuItem("", "Type this code into the station's printer setup page")
	systray.AddSeparator()
	newCode := systray.AddMenuItem("New pairing code", "Make a new code (an old pairing stops working)")
	printers := systray.AddMenuItem("Printers", "")
	printers.Disable()
	openStation := systray.AddMenuItem("Open Shipping Station", "")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "Stop the agent; labels then open in the browser instead")

	refresh := func() {
		if srv.cfg.Token != "" {
			paired.SetTitle("Paired with this computer's browser")
			codeItem.Hide()
		} else {
			paired.SetTitle("Not paired yet")
			code, ok := srv.pairing.Code()
			if !ok {
				code = srv.pairing.NewCode()
			}
			codeItem.SetTitle("Pairing code: " + code)
			codeItem.Show()
		}
		list, err := srv.spooler.ListPrinters()
		if err != nil || len(list) == 0 {
			printers.SetTitle("Printers: none found")
			return
		}
		names := ""
		for i, p := range list {
			if i > 0 {
				names += ", "
			}
			names += p.Name
			if p.State == "offline" {
				names += " (offline)"
			}
		}
		printers.SetTitle(fmt.Sprintf("Printers: %s", names))
	}
	srv.onPaired = refresh
	refresh()

	go func() {
		for {
			select {
			case <-newCode.ClickedCh:
				srv.cfg.Token = ""
				if err := srv.cfg.Save(); err != nil {
					log.Printf("save config: %v", err)
				}
				srv.pairing.NewCode()
				refresh()
			case <-codeItem.ClickedCh:
				refresh()
			case <-openStation.ClickedCh:
				if err := openURL(stationURL); err != nil {
					log.Printf("open station: %v", err)
				}
			case <-quit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}
