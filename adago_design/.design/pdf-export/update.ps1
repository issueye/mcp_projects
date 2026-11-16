# update.ps1 — 用最新构建产物覆盖已有节点 html
param(
  [Parameter(Mandatory = $true)][string]$Id,
  [Parameter(Mandatory = $true)][string]$Name,
  [int]$W = 0,
  [int]$H = 0,
  [string]$Auto = 'false',
  [string]$Base = 'http://127.0.0.1:8788'
)
$ErrorActionPreference = 'Stop'
$dir = $PSScriptRoot
$file = Join-Path $dir "build\$Name.html"
$html = [System.IO.File]::ReadAllText($file, [System.Text.Encoding]::UTF8)
$payload = [ordered]@{ html = $html; auto_height = ($Auto -eq 'true') }
if ($W -gt 0) { $payload.width = $W }
if ($H -gt 0) { $payload.height = $H }
$json = $payload | ConvertTo-Json -Depth 4 -Compress
$bytes = [System.Text.Encoding]::UTF8.GetBytes($json)
$r = Invoke-RestMethod -Uri "$Base/api/nodes/$Id" -Method Put -ContentType 'application/json; charset=utf-8' -Body $bytes
"updated $($r.node.id) 「$($r.node.title)」"
