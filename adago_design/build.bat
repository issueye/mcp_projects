@echo off
setlocal
set GOWORK=off
set CGO_ENABLED=0
REM windowsgui：无黑色控制台；日志写到 <data>\adago-design.log
go build -trimpath -ldflags="-s -w -H windowsgui" -o bin\adago-design.exe .
exit /b %errorlevel%
