# update.ps1 — 用最新构建产物覆盖已有节点的 html（尺寸交给 auto_height 回写）
# 用法: pwsh -File .design/native/update.ps1 -Id n-1794704583191-0002 -Name 01-main-light [-W 1440 -H 900]
param(
  [Parameter(Mandatory = $true)][string]$Id,
  [Parameter(Mandatory = $true)][string]$Name,
  [int]$W = 0,
  [int]$H = 0,
  [string]$Auto = 'true',
  [string]$Base = 'http://127.0.0.1:8788'
)
$ErrorActionPreference = 'Stop'
$dir = 'E:\codes\go_projects\mcp_projects\adago_design\.design\native'
$file = Join-Path $dir "build\$Name.html"
if (-not (Test-Path $file)) { throw "missing $file — 先跑 build.ps1" }
$html = [System.IO.File]::ReadAllText($file, [System.Text.Encoding]::UTF8)

$payload = [ordered]@{ html = $html; auto_height = ($Auto -eq 'true') }
if ($W -gt 0) { $payload.width = $W }
if ($H -gt 0) { $payload.height = $H }
$json = $payload | ConvertTo-Json -Depth 4 -Compress
$bytes = [System.Text.Encoding]::UTF8.GetBytes($json)
$r = Invoke-RestMethod -Uri "$Base/api/nodes/$Id" -Method Put -ContentType 'application/json; charset=utf-8' -Body $bytes
"updated $($r.node.id) 「$($r.node.title)」 html $([math]::Round($r.node.html.Length / 1024, 1)) KB"
