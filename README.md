# USCF Portal Print Agent

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
- PDF prints silently on any printer: on macOS through CUPS, on Windows with
  Windows' own PDF engine (no Adobe Reader or other PDF program needed). On a
  label printer it is fitted to a 4x6 label; on a page printer it prints at
  its real 4x6 size.
- ZPL (the station's optional fast mode) goes to the printer as raw bytes, so
  only Zebra-type printers take it.
- `/v1/status` reports each printer's driver (or make and model), its kind
  (`label`, `office`, or `raw` for a driverless pass-through queue) and
  whether it takes ZPL.
- Config (token, port, allowed origins): `~/Library/Application Support/USCF Portal Print Agent/config.json`
  on macOS, `%AppData%\USCF Portal Print Agent\config.json` on Windows.

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
# Windows with the icon + name in the .exe properties (needs go-winres):
# go install github.com/tc-hib/go-winres@v0.3.3 && go-winres make --in winres/winres.json --arch amd64
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-H windowsgui" -o uscf-print-agent.exe .
```

Unsigned installers: Windows shows SmartScreen once (More info → Run anyway); macOS blocks the .pkg until System Settings → Privacy & Security → Open Anyway (or `xattr -d com.apple.quarantine` on the file). An Apple Developer ID and a Windows code-signing certificate would remove both prompts.

Releases: push a tag `vX.Y.Z`; `.github/workflows/release.yml`
builds the Windows `.exe` and a macOS universal `.pkg` and attaches them to a
GitHub Release, which the station's printer setup page links to.

Supported systems: Windows 10 and 11 (64-bit), and macOS 10.15 Catalina or
later on Intel and Apple silicon. The release fails if the Mac binary asks for
a newer macOS than 10.15 on Intel or 11.0 on Apple silicon (the first macOS
those Macs came with).

## Updates

- Windows (0.4.0 and later) updates itself: a few minutes after starting, then
  every 6 hours, it reads the latest GitHub release, downloads the .exe for its
  processor, checks it against the release's `SHA256SUMS.txt`, and installs it
  (pairing and settings are kept). Downloads only come from GitHub.
- macOS needs an administrator password to install a package, so a Mac cannot
  update silently; the Shipping Station opens the update popup instead.
- To stop a bad release from spreading, delete it on GitHub: the previous
  release becomes "latest" again and agents stop updating to it.

