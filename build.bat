@echo off
echo Building CAN Bus Reader for Windows...

echo.
echo Building 64-bit version (default)...
go build -o canbus-reader-64.exe

echo.
echo Building 32-bit version (for 32-bit ControlCAN.dll)...
set GOARCH=386
go build -o canbus-reader-32.exe
set GOARCH=amd64

echo.
echo Building Linux version...
set GOOS=linux
set GOARCH=amd64
go build -o canbus-reader-linux
set GOOS=windows

echo.
echo Done!
echo.
echo Files created:
echo   canbus-reader-64.exe    - 64-bit Windows executable
echo   canbus-reader-32.exe    - 32-bit Windows executable (for 32-bit DLLs)
echo   canbus-reader-linux.lexe     - Linux executable
echo.
echo Note: If ControlCAN.dll is 32-bit, use canbus-reader-32.exe
echo       If ControlCAN.dll is 64-bit, use canbus-reader-64.exe