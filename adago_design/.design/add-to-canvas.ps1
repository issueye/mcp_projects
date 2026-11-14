# add-to-canvas.ps1 — 把 .design/build/<Name>.html 作为节点摆到画布项目里
# 用法: pwsh -File add-to-canvas.ps1 -Name 01-main-light -Title "01 主窗 · 浅色" [-Kind screen] [-W 1352 -H 932 -X 120 -Y 140]
param(
  [Parameter(Mandatory = $true)][string]$Name,
  [Parameter(Mandatory = $true)][string]$Title,
  [string]$Kind = 'screen',
  [int]$W = 1352,
  [int]$H = 932,
  [double]$X = -1,
  [double]$Y = -1,
  [string]$Tags = '',
  [string]$Base = 'http://127.0.0.1:8788',
  [string]$Project = 'p-1794648772080-0001'
)
$ErrorActionPreference = 'Stop'
$dir = 'E:\codes\go_projects\mcp_projects\adago_design\.design'
$file = Join-Path $dir "build\$Name.html"
if (-not (Test-Path $file)) { throw "missing $file — 先跑 build-node.ps1" }
$html = [System.IO.File]::ReadAllText($file, [System.Text.Encoding]::UTF8)

$payload = [ordered]@{
  project  = $Project
  title    = $Title
  system   = 'inkwell'
  kind     = $Kind
  html     = $html
  artboard = 'desktop-app'
  width    = $W
  height   = $H
  tags     = @('browser-use', 'inkwell', 'redesign') + @($Tags -split ',' | Where-Object { $_ })
}
if ($X -ge 0 -and $Y -ge 0) { $payload.x = $X; $payload.y = $Y }
$json = $payload | ConvertTo-Json -Depth 5 -Compress
$bytes = [System.Text.Encoding]::UTF8.GetBytes($json)
$r = Invoke-RestMethod -Uri "$Base/api/nodes" -Method Post -ContentType 'application/json; charset=utf-8' -Body $bytes
"added node $($r.node.id) 「$($r.node.title)」 ($($r.node.width)x$($r.node.height) @ $([math]::Round($r.node.x)),$([math]::Round($r.node.y)))"
