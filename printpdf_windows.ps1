# Prints one PDF label silently on a named Windows printer, with no PDF program
# installed: Windows' own PDF engine (Windows.Data.Pdf) draws each page at the
# printer's own resolution and System.Drawing sends it to the printer driver.
# On a label printer the page is sized and fitted to a 4x6 label; on a page
# printer the label prints at its real 4x6 size in the top-left corner.
param([string]$Pdf, [string]$Printer, [int]$Label)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing
Add-Type -AssemblyName System.Runtime.WindowsRuntime
[void][Windows.Storage.StorageFile, Windows.Storage, ContentType = WindowsRuntime]
[void][Windows.Data.Pdf.PdfDocument, Windows.Data.Pdf, ContentType = WindowsRuntime]
[void][Windows.Data.Pdf.PdfPageRenderOptions, Windows.Data.Pdf, ContentType = WindowsRuntime]
[void][Windows.Storage.Streams.InMemoryRandomAccessStream, Windows.Storage.Streams, ContentType = WindowsRuntime]

$ext = [System.WindowsRuntimeSystemExtensions].GetMethods()
$asTaskOp = $ext | Where-Object { $_.Name -eq 'AsTask' -and $_.IsGenericMethod -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation`1' } | Select-Object -First 1
$asTaskAct = $ext | Where-Object { $_.Name -eq 'AsTask' -and -not $_.IsGenericMethod -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncAction' } | Select-Object -First 1
function Wait-Op($op, [Type]$type) { $t = $asTaskOp.MakeGenericMethod($type).Invoke($null, @($op)); $t.Wait(); return $t.Result }
function Wait-Act($act) { $asTaskAct.Invoke($null, @($act)).Wait() }

$pd = New-Object System.Drawing.Printing.PrintDocument
$pd.PrinterSettings.PrinterName = $Printer
if (-not $pd.PrinterSettings.IsValid) { throw "No printer named '$Printer' is installed." }
$pd.DocumentName = 'USCF label'
$pd.PrintController = New-Object System.Drawing.Printing.StandardPrintController   # no progress window
$pd.DefaultPageSettings.Margins = New-Object System.Drawing.Printing.Margins(0, 0, 0, 0)
if ($Label -eq 1) {
  # Use the driver's own 4x6 paper size when it has one (sizes are in 1/100 inch).
  $size = $pd.PrinterSettings.PaperSizes | Where-Object { [math]::Abs($_.Width - 400) -le 12 -and [math]::Abs($_.Height - 600) -le 12 } | Select-Object -First 1
  if (-not $size) { $size = New-Object System.Drawing.Printing.PaperSize('4x6', 400, 600) }
  $pd.DefaultPageSettings.PaperSize = $size
  $pd.DefaultPageSettings.Landscape = $false
}

# Draw the label at the printer's real resolution so bars stay crisp (no resampling).
$measure = $pd.PrinterSettings.CreateMeasurementGraphics($pd.DefaultPageSettings)
$dpi = [int]$measure.DpiX
$measure.Dispose()
if ($dpi -lt 150) { $dpi = 203 }

$file = Wait-Op ([Windows.Storage.StorageFile]::GetFileFromPathAsync($Pdf)) ([Windows.Storage.StorageFile])
$doc = Wait-Op ([Windows.Data.Pdf.PdfDocument]::LoadFromFileAsync($file)) ([Windows.Data.Pdf.PdfDocument])
$script:pages = New-Object System.Collections.ArrayList
for ($i = 0; $i -lt $doc.PageCount; $i++) {
  $page = $doc.GetPage([uint32]$i)
  $opts = New-Object Windows.Data.Pdf.PdfPageRenderOptions
  # Page size is in 1/96 inch.
  $opts.DestinationWidth = [uint32][math]::Round($page.Size.Width * $dpi / 96)
  $opts.DestinationHeight = [uint32][math]::Round($page.Size.Height * $dpi / 96)
  $stream = New-Object Windows.Storage.Streams.InMemoryRandomAccessStream
  Wait-Act ($page.RenderToStreamAsync($stream, $opts))
  $stream.Seek(0)
  $net = [System.IO.WindowsRuntimeStreamExtensions]::AsStreamForRead($stream)
  $ms = New-Object System.IO.MemoryStream
  $net.CopyTo($ms)
  $ms.Position = 0
  [void]$script:pages.Add([System.Drawing.Image]::FromStream($ms))
  $page.Dispose()
}
if ($script:pages.Count -eq 0) { throw 'The label PDF has no pages.' }

$script:n = 0
$script:dpi = $dpi
$script:label = $Label
$pd.add_PrintPage({
  param($sender, $e)
  $img = $script:pages[$script:n]
  $g = $e.Graphics
  $g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::NearestNeighbor
  $g.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::Half
  # Sizes in 1/100 inch; the page origin sits at the printer's hard margin.
  $w = $img.Width * 100.0 / $script:dpi
  $h = $img.Height * 100.0 / $script:dpi
  $pw = $e.PageBounds.Width; $ph = $e.PageBounds.Height
  if ($script:label -eq 1) {
    $scale = [math]::Min($pw / $w, $ph / $h)
    $x = ($pw - $w * $scale) / 2; $y = ($ph - $h * $scale) / 2
  } else {
    $scale = [math]::Min(1.0, [math]::Min(($pw - 50) / $w, ($ph - 50) / $h))
    $x = 25; $y = 25
  }
  $x -= $e.PageSettings.HardMarginX; $y -= $e.PageSettings.HardMarginY
  $g.DrawImage($img, [single]$x, [single]$y, [single]($w * $scale), [single]($h * $scale))
  $script:n++
  $e.HasMorePages = $script:n -lt $script:pages.Count
})
$pd.Print()
foreach ($img in $script:pages) { $img.Dispose() }
