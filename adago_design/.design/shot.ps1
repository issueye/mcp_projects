# shot.ps1 — 用 Chrome headless 给任意 HTML 文件出图（校验设计稿的真实渲染）
# 用法: pwsh -File .design/shot.ps1 -Html .design/build/01-main-light.html -Out .design/shots/01.png [-W 1352 -H 932]
param(
  [Parameter(Mandatory = $true)][string]$Html,
  [Parameter(Mandatory = $true)][string]$Out,
  [int]$W = 1352,
  [int]$H = 932
)
$ErrorActionPreference = 'Stop'
$chrome = "C:\Program Files\Google\Chrome\Application\chrome.exe"
$htmlPath = (Resolve-Path $Html).Path
if ([System.IO.Path]::IsPathRooted($Out)) { $outPath = [System.IO.Path]::GetFullPath($Out) }
else { $outPath = [System.IO.Path]::GetFullPath((Join-Path 'E:\codes\go_projects\mcp_projects\adago_design' $Out)) }
New-Item -ItemType Directory -Force -Path (Split-Path $outPath) | Out-Null
if (Test-Path $outPath) { Remove-Item $outPath -Force }
$url = [System.Uri]::new($htmlPath).AbsoluteUri
$args = @(
  '--headless=new', '--disable-gpu', '--hide-scrollbars', '--force-device-scale-factor=1',
  '--allow-file-access-from-files', "--window-size=$W,$H",
  "--screenshot=$outPath", '--virtual-time-budget=4000', $url
)
$p = Start-Process -FilePath $chrome -ArgumentList $args -PassThru -Wait -WindowStyle Hidden
if (-not (Test-Path $outPath)) { throw "screenshot failed (chrome exit $($p.ExitCode))" }
"shot $outPath ($([math]::Round((Get-Item $outPath).Length/1024,1)) KB)"
