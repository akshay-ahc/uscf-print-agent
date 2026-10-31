# USCF Print Agent

A small Go program for the shipping PC. It lets the Shipping Station page (in
Chrome) see the computer's printers and print shipping labels on them
silently, with nothing leaving the PC and no third-party service.

```
Shipping Station page (HTTPS)  →  http://127.0.0.1:17870  (this agent)
  GET  /v1/status   { version, paired, printers: [{ name, default, state }] }
  POST /v1/pair     { code } → { token }        one-time, code from the tray icon
  POST /v1/print    { printer, format: PDF|ZPL, data: base64 } → { ok, job_id }
                    Authorization: Bearer <token>
```

- Listens on `127.0.0.1` only. Only the portal origins (production and
  sandbox) pass CORS; any other website is refused with 403.
- Pairing: the tray shows a 6-digit code (10-minute life, single use). The
  station page exchanges it for a random token, which it must send on every
  print. "New pairing code" in the tray forgets the old token.
- ZPL goes to the printer as raw bytes (Zebra). PDF goes through the OS print
  path (CUPS on macOS; on Windows the program registered for PDFs, e.g. Adobe
  Reader; Edge cannot, so use ZPL for a Zebra).
- Config (token, port, allowed origins): `~/Library/Application Support/USCF Print Agent/config.json`
  on macOS, `%AppData%\USCF Print Agent\config.json` on Windows.

## Run

```
uscf-print-agent               tray icon + local API; registers itself to start at login (LaunchAgent / HKCU Run key)
uscf-print-agent --install     same, and confirm the login item on the console
uscf-print-agent --uninstall   stop starting at login
uscf-print-agent --no-tray     headless; prints the pairing code to the log
```

## Build

Go 1.22+. `go test ./...` runs the API and parsing tests.

```

go build -ldflags "-X main.Version=0.1.0" -o uscf-print-agent .                 # this machine
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-H windowsgui" -o uscf-print-agent.exe .
```

Unsigned installers: Windows shows SmartScreen once (More info → Run anyway); macOS blocks the .pkg until System Settings → Privacy & Security → Open Anyway (or `xattr -d com.apple.quarantine` on the file). An Apple Developer ID and a Windows code-signing certificate would remove both prompts.

Releases: push a tag `vX.Y.Z`; `.github/workflows/release.yml`
builds the Windows `.exe` and a macOS universal `.pkg` and attaches them to a
GitHub Release, which the station's printer setup page links to.
