# Icon sources

- `icon.svg` – the app icon (Windows .exe, installer window, website).
- `tray-template.svg` – the macOS menu-bar glyph (black on transparent; macOS recolours it).

Rasterize after a change (needs Node with `sharp`, e.g. from the portal repo's node_modules):

```
node tools/mkicons.js .   # writes icon-*.png, ../icon.png, ../tray-template.png, pkg-background.png
node tools/mkico.js       # writes ../icon.ico from icon-16…256.png
```
