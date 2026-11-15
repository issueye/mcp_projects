# build.ps1 — 把 .design/icons/screens/<Name>.html 组装成可入库节点 HTML
# 用法: pwsh -File .design/icons/build.ps1 -Name 01-default
# 与 .design/systems 共用 kit.css / icons.svg / backdrop.html，只在本目录追加图标专用样式。
param(
  [Parameter(Mandatory = $true)][string]$Name,
  [string]$OutDir = 'build'
)
$ErrorActionPreference = 'Stop'
$dir = 'E:\codes\go_projects\mcp_projects\adago_design\.design\icons'
$src = Join-Path $dir "screens\$Name.html"
if (-not (Test-Path $src)) { throw "missing $src" }

$kit = [System.IO.File]::ReadAllText((Join-Path $dir '..\systems\kit.css'), [System.Text.Encoding]::UTF8)
$extra = [System.IO.File]::ReadAllText((Join-Path $dir 'icons.css'), [System.Text.Encoding]::UTF8)
$icons = [System.IO.File]::ReadAllText((Join-Path $dir '..\systems\icons.svg'), [System.Text.Encoding]::UTF8)
$body = [System.IO.File]::ReadAllText($src, [System.Text.Encoding]::UTF8)

for ($i = 0; $i -lt 8; $i++) {
  $ms = [regex]::Matches($body, '<!--INCLUDE:([^>]+?)-->')
  if ($ms.Count -eq 0) { break }
  foreach ($m in $ms) {
    $f = [System.IO.Path]::GetFullPath((Join-Path $dir $m.Groups[1].Value))
    if (-not (Test-Path $f)) { throw "include missing: $f" }
    $body = $body.Replace($m.Value, [System.IO.File]::ReadAllText($f, [System.Text.Encoding]::UTF8))
  }
}
if ($body -match '<!--INCLUDE:') { throw 'include 嵌套超过 8 层，检查是否循环引用' }

$css = $kit + "`n" + $extra
$csp = '<meta http-equiv="Content-Security-Policy" content="default-src ''none''; style-src ''unsafe-inline''; img-src data:; font-src data:">'
$doc = @"
<!DOCTYPE html>
<html lang="zh-CN"><head>
$csp<meta charset="utf-8"><meta name="viewport" content="width=1440">
<title>$Name</title>
<style>
$css
</style></head>
<body>
$icons
$body
</body></html>
"@
$outPath = Join-Path (Join-Path $dir $OutDir) "$Name.html"
New-Item -ItemType Directory -Force -Path (Split-Path $outPath) | Out-Null
[System.IO.File]::WriteAllText($outPath, $doc, (New-Object System.Text.UTF8Encoding($false)))
"built $outPath ($([math]::Round($doc.Length / 1024, 1)) KB)"
exit 0
