// Builds icon.ico (PNG-compressed entries) from assets/icon-<size>.png. Run from the repo root.
const fs = require('fs'); const sizes = [16, 24, 32, 48, 64, 128, 256]
const imgs = sizes.map(s => ({ s, buf: fs.readFileSync(`assets/icon-${s}.png`) }))
const hdr = Buffer.alloc(6); hdr.writeUInt16LE(0, 0); hdr.writeUInt16LE(1, 2); hdr.writeUInt16LE(imgs.length, 4)
let off = 6 + 16 * imgs.length; const dirs = []
for (const { s, buf } of imgs) { const d = Buffer.alloc(16); d[0] = s === 256 ? 0 : s; d[1] = s === 256 ? 0 : s; d.writeUInt16LE(1, 4); d.writeUInt16LE(32, 6); d.writeUInt32LE(buf.length, 8); d.writeUInt32LE(off, 12); off += buf.length; dirs.push(d) }
fs.writeFileSync('icon.ico', Buffer.concat([hdr, ...dirs, ...imgs.map(i => i.buf)])); console.log('icon.ico written')
