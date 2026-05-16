@echo off
REM install.bat — download and install the skillpack binary on Windows
REM Usage:
REM   curl -fsSL https://raw.githubusercontent.com/bmaltais/skillpack/main/install.bat -o /tmp/skillpack-install.bat
REM   cmd /c skillpack-install.bat
REM
REM One-liner (PowerShell):
REM   powershell -NoProfile -ExecutionPolicy Bypass -Command "[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12; Invoke-WebRequest -Uri 'https://raw.githubusercontent.com/bmaltais/skillpack/main/install.bat' -OutFile skillpack-install.bat; cmd /c skillpack-install.bat"

setlocal EnableDelayedExpansion

set "REPO=bmaltais/skillpack"
set "BINARY=skillpack"
set "BASE_URL=https://github.com/%REPO%/releases/latest/download"

REM ── Detect arch ──────────────────────────────────────────────────────────────────
set "ARCH=amd64"
if "%PROCESSOR_ARCHITECTURE%"=="ARM64" set "ARCH=arm64"

REM ── Choose install directory ────────────────────────────────────────────────────
REM User-local, no admin needed. Same convention as Linux (~/.local/bin).
set "install_dir=%USERPROFILE%\.local\bin"
if not exist "!install_dir!" (
    mkdir "!install_dir!" >nul 2>&1
)

REM ── Build asset URL ──────────────────────────────────────────────────────────────
set "asset=%BINARY%-windows-%ARCH%.exe"
set "url=%BASE_URL%/%asset%"
set "dest=%install_dir%\%BINARY%.exe"

REM ── Download (try curl, fall back to PowerShell) ──────────────────────────────────
echo Downloading %asset% ...
echo   to %dest%

curl -fsSL "%url%" -o "!dest!" 2>nul
if errorlevel 1 (
    powershell -NoProfile -ExecutionPolicy Bypass -Command "Invoke-WebRequest -Uri '%url%' -OutFile '!dest!'"
    if errorlevel 1 (
        echo.
        echo error: download failed.
        echo       Check your internet connection or download manually from:
        echo       https://github.com/%REPO%/releases/latest
        echo.
        pause
        exit /b 1
    )
)

REM ── Try to add install_dir to PATH (inline PowerShell, no admin needed) ──────────
REM Append to the user-level PATH environment variable.
set "PATH_UPDATED=0"
powershell -NoProfile -ExecutionPolicy Bypass -^
    "try { [Environment]::SetEnvironmentVariable(^'Path^', [Environment]::GetEnvironmentVariable(^'Path^', ^'User^') + ^';%install_dir%^', ^'User^'); exit 0 } catch { exit 1 }" 2>nul
if !errorlevel! equ 0 set "PATH_UPDATED=1"

echo.
echo skillpack installed to !dest!
echo.
if "!PATH_UPDATED!"=="1" (
    echo NOTE: PATH has been updated. Restart your terminal to use skillpack.
) else (
    echo NOTE: PATH was not updated automatically.
    echo       Add it manually by running:
    echo.
    echo         set PATH="%install_dir%;%PATH%"
    echo.
    echo       Then restart your terminal.
)
echo.
pause
