package main

import _ "embed"

// Tray icons. macOS gets a monochrome "template" image (the menu bar recolours
// it for light/dark); Windows gets the full-colour .ico. The source SVGs are in
// assets/, rasterized by assets/README.md's steps.
//
//go:embed tray-template.png
var trayIconPNG []byte

//go:embed icon.ico
var trayIconICO []byte
