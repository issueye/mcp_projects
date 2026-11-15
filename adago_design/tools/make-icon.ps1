# make-icon.ps1 — 重新生成图标资源（母版 assets/icon.png → ico/窗口/托盘 → syso）
# 用法：pwsh -File tools/make-icon.ps1 [-Windres C:\path\to\windres.exe]
param(
  [string]$Windres = 'windres'
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
  & go run ./tools/icopack
  if ($LASTEXITCODE -ne 0) { throw 'icopack failed' }
  if (-not (Get-Command $Windres -ErrorAction SilentlyContinue)) {
    throw "windres not found ($Windres); install MinGW-w64 or pass -Windres <path>"
  }
  & $Windres -F pe-x86-64 -I (Join-Path $root 'assets') -i (Join-Path $root 'assets\icon.rc') -o (Join-Path $root 'rsrc_windows_amd64.syso') -O coff
  if ($LASTEXITCODE -ne 0) { throw 'windres failed' }
  'icon resources rebuilt: assets/icon.ico + rsrc_windows_amd64.syso'
} finally {
  Pop-Location
}