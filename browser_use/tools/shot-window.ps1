# shot-window.ps1 — 截取指定进程主窗口（用于验证 mygo 原生界面真实渲染）
param(
  [Parameter(Mandatory = $true)][string]$ProcessName,
  [string]$Out = 'E:\codes\go_projects\mcp_projects\browser_use\docs\ui-inkwell.png',
  [int]$ForceWidth = 0,
  [int]$ForceHeight = 0
)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class WinCap {
  [StructLayout(LayoutKind.Sequential)] public struct RECT { public int Left, Top, Right, Bottom; }
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr hWnd, out RECT r);
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);
  [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr hWnd, int n);
  [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr hWnd, IntPtr after, int x, int y, int cx, int cy, uint flags);
}
"@
$proc = Get-Process -Name $ProcessName -ErrorAction Stop | Where-Object { $_.MainWindowHandle -ne 0 } | Select-Object -First 1
if (-not $proc) { throw "no window for $ProcessName" }
$h = $proc.MainWindowHandle
[WinCap]::ShowWindow($h, 9) | Out-Null   # SW_RESTORE
Start-Sleep -Milliseconds 250
if ($ForceWidth -gt 0 -and $ForceHeight -gt 0) {
  # flags: SWP_NOZORDER(4) | SWP_NOACTIVATE(16)
  [WinCap]::SetWindowPos($h, [IntPtr]::Zero, 40, 40, $ForceWidth, $ForceHeight, 0x14) | Out-Null
  Start-Sleep -Milliseconds 900
}
[WinCap]::SetForegroundWindow($h) | Out-Null
Start-Sleep -Milliseconds 700
$r = New-Object WinCap+RECT
[WinCap]::GetWindowRect($h, [ref]$r) | Out-Null
$w = $r.Right - $r.Left; $ht = $r.Bottom - $r.Top
"window rect $($r.Left),$($r.Top) ${w}x${ht}"
$bmp = New-Object System.Drawing.Bitmap($w, $ht)
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.CopyFromScreen($r.Left, $r.Top, 0, 0, (New-Object System.Drawing.Size($w, $ht)))
$g.Dispose()
New-Item -ItemType Directory -Force -Path (Split-Path $Out) | Out-Null
$bmp.Save($Out, [System.Drawing.Imaging.ImageFormat]::Png)
$bmp.Dispose()
"saved $Out ($([math]::Round((Get-Item $Out).Length/1024,1)) KB)"
