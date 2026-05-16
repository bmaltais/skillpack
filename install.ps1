# install.ps1 — download and install the skillpack binary on Windows
# Usage:
#   powershell -NoProfile -ExecutionPolicy Bypass -Command "[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12; Invoke-WebRequest -Uri 'https://raw.githubusercontent.com/bmaltais/skillpack/main/install.ps1' -OutFile skillpack-install.ps1; .\skillpack-install.ps1"

$ErrorActionPreference = "Stop"

$REPO = "bmaltais/skillpack"
$BINARY = "skillpack"
$BASE_URL = "https://github.com/$REPO/releases/latest/download"

# ── Detect arch ──────────────────────────────────────────────────────────────────
$arch = "amd64"
if ([Environment]::Is64BitOperatingSystem -and (Get-CimInstance -ClassName Win32_Processor -Property Architecture -ErrorAction SilentlyContinue).Architecture -eq 12) {
    $arch = "arm64"
} elseif ([Environment]::Is64BitOperatingSystem -eq $false) {
    $arch = "amd64"
}

# ── Choose install directory ────────────────────────────────────────────────────
$installDir = Join-Path $env:USERPROFILE ".local\bin"
if (-not (Test-Path $installDir)) {
    New-Item -ItemType Directory -Path $installDir -Force | Out-Null
}

# ── Build asset URL ──────────────────────────────────────────────────────────────
$asset = "$BINARY-windows-$arch.exe"
$url = "$BASE_URL/$asset"
$dest = Join-Path $installDir "$BINARY.exe"
$tmpDest = Join-Path $installDir ".skillpack-tmp.exe"

# ── Download ─────────────────────────────────────────────────────────────────────
Write-Host "Downloading $asset ..." -ForegroundColor Cyan
Write-Host "  to $dest" -ForegroundColor Cyan

try {
    Invoke-WebRequest -Uri $url -OutFile $tmpDest -UseBasicParsing
} catch {
    Write-Host "`nerror: download failed." -ForegroundColor Red
    Write-Host "       Check your internet connection or download manually from:" -ForegroundColor Red
    Write-Host "       https://github.com/$REPO/releases/latest" -ForegroundColor Red
    exit 1
}

# Atomically replace the installed binary
Move-Item -Path $tmpDest -Destination $dest -Force

# ── Try to add install_dir to PATH ───────────────────────────────────────────────
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$pathUpdated = $true
if ($userPath -notlike "*$installDir*") {
    try {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$installDir", "User")
    } catch {
        $pathUpdated = $false
    }
}

Write-Host "`nskillpack installed to $dest" -ForegroundColor Green

if ($pathUpdated) {
    Write-Host "`nNOTE: PATH has been updated. Restart your terminal to use skillpack." -ForegroundColor Yellow
} else {
    Write-Host "`nNOTE: PATH was not updated automatically." -ForegroundColor Yellow
    Write-Host "      Add it manually by running:" -ForegroundColor Yellow
    Write-Host "" -ForegroundColor Yellow
    Write-Host "        $env:PATH = '$installDir;$env:PATH'" -ForegroundColor Yellow
    Write-Host "" -ForegroundColor Yellow
    Write-Host "      Then restart your terminal." -ForegroundColor Yellow
}
