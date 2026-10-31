package main

import _ "embed"

// The tray icon: the portal's icon. Windows wants an .ico, macOS/Linux a PNG.
//
//go:embed icon.png
var trayIconPNG []byte

//go:embed icon.ico
var trayIconICO []byte
