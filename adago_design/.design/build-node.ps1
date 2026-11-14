# build-node.ps1 — 把 .design/screens/<name>.html（只含窗口 body）组装成可入库节点 HTML
# 用法: pwsh -File .design/build-node.ps1 -Name 01-main-light
# 支持占位：<!--SITE--> 注入 site-harbor.html；<!--INCLUDE:文件名--> 注入 .design/文件名
param(
  [Parameter(Mandatory = $true)][string]$Name
)
$ErrorActionPreference = 'Stop'
$dir = 'E:\codes\go_projects\mcp_projects\adago_design\.design'
$src = Join-Path $dir "screens\$Name.html"
if (-not (Test-Path $src)) { throw "missing $src" }
$css = [System.IO.File]::ReadAllText((Join-Path $dir 'kit.css'), [System.Text.Encoding]::UTF8)
$icons = [System.IO.File]::ReadAllText((Join-Path $dir 'icons.svg'), [System.Text.Encoding]::UTF8)
$body = [System.IO.File]::ReadAllText($src, [System.Text.Encoding]::UTF8)

$site = Join-Path $dir 'site-harbor.html'
if ($body.Contains('<!--SITE-->') -and (Test-Path $site)) {
  $body = $body.Replace('<!--SITE-->', [System.IO.File]::ReadAllText($site, [System.Text.Encoding]::UTF8))
}
foreach ($m in [regex]::Matches($body, '<!--INCLUDE:([^>]+?)-->')) {
  $f = Join-Path $dir $m.Groups[1].Value
  if (Test-Path $f) {
    $body = $body.Replace($m.Value, [System.IO.File]::ReadAllText($f, [System.Text.Encoding]::UTF8))
  }
}

$dark = if ($body -match 'stage theme-dark') { ' class="theme-dark"' } else { '' }
$csp = '<meta http-equiv="Content-Security-Policy" content="default-src ''none''; style-src ''unsafe-inline''; img-src data:; font-src data:">'
$doc = @"
<!DOCTYPE html>
<html lang="zh-CN"><head>
$csp<meta charset="utf-8"><meta name="viewport" content="width=1352">
<style>
$css
</style></head>
<body$dark>
$icons
$body
</body></html>
"@
$outDir = Join-Path $dir 'build'
New-Item -ItemType Directory -Force -Path $outDir | Out-Null
$out = Join-Path $outDir "$Name.html"
[System.IO.File]::WriteAllText($out, $doc, (New-Object System.Text.UTF8Encoding($false)))
"built $out ($([math]::Round($doc.Length/1024,1)) KB)"
exit 0
