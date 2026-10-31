const sharp = require(process.env.SHARP_PATH || 'sharp')
const fs = require('fs'); const path = require('path')
const dir = process.argv[2]
async function main() {
  const svg = fs.readFileSync(path.join(dir, 'assets/icon.svg'))
  for (const s of [16, 24, 32, 48, 64, 128, 256, 512, 1024]) {
    await sharp(svg, { density: 72 * (s / 256) * 4 }).resize(s, s).png().toFile(path.join(dir, `assets/icon-${s}.png`))
  }
  fs.copyFileSync(path.join(dir, 'assets/icon-256.png'), path.join(dir, 'icon.png'))
  const tray = fs.readFileSync(path.join(dir, 'assets/tray-template.svg'))
  await sharp(tray, { density: 288 }).resize(44, 44).png().toFile(path.join(dir, 'tray-template.png'))
  // Installer background for the macOS .pkg window (left panel, 620x418 is the classic size).
  const bgSvg = Buffer.from(`<svg xmlns="http://www.w3.org/2000/svg" width="620" height="418"><rect width="620" height="418" fill="#ffffff"/></svg>`)
  const icon = await sharp(svg, { density: 72 * 4 }).resize(140, 140).png().toBuffer()
  await sharp(bgSvg).composite([{ input: icon, left: 40, top: 230 }]).png().toFile(path.join(dir, 'assets/pkg-background.png'))
  console.log('ok')
}
main().catch(e => { console.error(e); process.exit(1) })
