# Pling Agent installer for Windows
# Run: irm https://raw.githubusercontent.com/Jeramo/pling-agent/main/install.ps1 | iex

$ErrorActionPreference = "Stop"
$repo = "Jeramo/pling-agent"
$installDir = "$env:ProgramFiles\Pling Agent"
$configDir = "$env:ProgramData\pling-agent"
$serviceName = "PlingAgent"

# Detect architecture
$arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { Write-Error "32-bit Windows not supported"; exit 1 }

Write-Host "Detected: windows/$arch" -ForegroundColor Cyan

# Get latest release URL
$release = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest"
$asset = $release.assets | Where-Object { $_.name -eq "pling-agent-windows-$arch.exe" } | Select-Object -First 1
if (-not $asset) {
    Write-Error "No release found for windows/$arch"
    exit 1
}

# Download
Write-Host "Downloading $($asset.browser_download_url)..."
New-Item -ItemType Directory -Force -Path $installDir | Out-Null
$binPath = Join-Path $installDir "pling-agent.exe"
Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $binPath

# Config
$configFile = Join-Path $configDir "config.toml"
if (-not (Test-Path $configFile)) {
    New-Item -ItemType Directory -Force -Path $configDir | Out-Null
    $token = Read-Host "Enter your Pling API token"
    @"
token = "$token"
api_url = "https://agent.plingpush.com"
metrics_interval = 60
allow_remote_commands = true
webui_port = 9876
"@ | Set-Content $configFile
    Write-Host "Config written to $configFile"
}

# Install as Windows Service using sc.exe
$existing = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
if ($existing) {
    Write-Host "Stopping existing service..."
    Stop-Service -Name $serviceName -Force -ErrorAction SilentlyContinue
    sc.exe delete $serviceName | Out-Null
    Start-Sleep -Seconds 2
}

Write-Host "Creating Windows service..."
sc.exe create $serviceName binPath= "`"$binPath`"" start= auto | Out-Null
sc.exe description $serviceName "Pling Agent - 24/7 server monitoring and scheduled commands" | Out-Null
sc.exe failure $serviceName reset= 60 actions= restart/10000/restart/30000/restart/60000 | Out-Null
Start-Service -Name $serviceName

$ip = (Get-NetIPAddress -AddressFamily IPv4 | Where-Object { $_.InterfaceAlias -notmatch "Loopback" -and $_.PrefixOrigin -ne "WellKnown" } | Select-Object -First 1).IPAddress
Write-Host ""
Write-Host "Done! Pling Agent is running." -ForegroundColor Green
Write-Host "Agent settings: http://${ip}:9876" -ForegroundColor Cyan
