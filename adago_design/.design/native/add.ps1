# add.ps1 — 把 .design/native/build/<Name>.html 作为节点摆到画布项目里
# 用法:
#   pwsh -File .design/native/add.ps1 -Name 01-main-light -Title "01 主窗 · 浅色" -W 1440 -H 900 -Artboard desktop-app
#   pwsh -File .design/native/add.ps1 -Name 00-note -Title "00 设计说明" -W 980 -H 1160 -Kind note -Artboard ''
param(
  [Parameter(Mandatory = $true)][string]$Name,
  [Parameter(Mandatory = $true)][string]$Title,
  [string]$Kind = 'screen',
  [int]$W = 1440,
  [int]$H = 900,
  [double]$X = -1,
  [double]$Y = -1,
  [string]$Artboard = 'desktop-app',
  [string]$Tags = 'native-shell,adago-design',
  [string]$Base = 'http://127.0.0.1:8788',
  [Parameter(Mandatory = $true)][string]$Project
)
$ErrorActionPreference = 'Stop'
$dir = 'E:\codes\go_projects\mcp_projects\adago_design\.design\native'
$file = Join-Path $dir "build\$Name.html"
if (-not (Test-Path $file)) { throw "missing $file — 先跑 build.ps1" }
$html = [System.IO.File]::ReadAllText($file, [System.Text.Encoding]::UTF8)

$payload = [ordered]@{
  project = $Project
  title   = $Title
  system  = 'paper-frame'
  kind    = $Kind
  html    = $html
  width   = $W
  height  = $H
  tags    = @($Tags -split ',' | Where-Object { $_ })
}
if ($Artboard -ne '') { $payload.artboard = $Artboard }
if ($X -ge 0 -and $Y -ge 0) { $payload.x = $X; $payload.y = $Y }
$json = $payload | ConvertTo-Json -Depth 5 -Compress
$bytes = [System.Text.Encoding]::UTF8.GetBytes($json)
$r = Invoke-RestMethod -Uri "$Base/api/nodes" -Method Post -ContentType 'application/json; charset=utf-8' -Body $bytes
"added $($r.node.id) 「$($r.node.title)」 $($r.node.width)x$($r.node.height) @ $([math]::Round($r.node.x)),$([math]::Round($r.node.y))"
