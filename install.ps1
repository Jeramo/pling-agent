# Pling installer for Windows
# Run: irm https://raw.githubusercontent.com/Jeramo/pling-agent/main/install.ps1 | iex

$ErrorActionPreference = "Stop"
$repo = "Jeramo/pling-agent"
$installDir = "$env:ProgramFiles\Pling"
$configDir = "$env:ProgramData\pling"
$serviceName = "Pling"
$legacyService = "PlingAgent"
$legacyInstallDir = "$env:ProgramFiles\Pling Agent"
$legacyConfigDir = "$env:ProgramData\pling-agent"

# Detect architecture
$cpuArch = $env:PROCESSOR_ARCHITECTURE
$arch = if ($cpuArch -eq "ARM64") { "arm64" } elseif ([Environment]::Is64BitOperatingSystem) { "amd64" } else { Write-Error "32-bit Windows not supported"; exit 1 }

Write-Host "Detected: windows/$arch" -ForegroundColor Cyan

# Stop legacy service if present
$legacy = Get-Service -Name $legacyService -ErrorAction SilentlyContinue
if ($legacy) {
    Write-Host "Stopping legacy PlingAgent service..."
    Stop-Service -Name $legacyService -Force -ErrorAction SilentlyContinue
    sc.exe delete $legacyService | Out-Null
    Start-Sleep -Seconds 2
}

# Get latest release URL
$release = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest"
$asset = $release.assets | Where-Object { $_.name -eq "pling-windows-$arch.exe" } | Select-Object -First 1
if (-not $asset) {
    Write-Error "No release found for windows/$arch"
    exit 1
}

# Download
Write-Host "Downloading $($asset.browser_download_url)..."
New-Item -ItemType Directory -Force -Path $installDir | Out-Null
$binPath = Join-Path $installDir "pling.exe"
Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $binPath

# Verify SHA-256 checksum if published
$checksumAsset = $release.assets | Where-Object { $_.name -eq "pling-windows-$arch.exe.sha256" } | Select-Object -First 1
if ($checksumAsset) {
    $checksumContent = (Invoke-WebRequest -Uri $checksumAsset.browser_download_url).Content
    $expected = ($checksumContent -split '\s')[0].Trim().ToLower()
    $actual = (Get-FileHash -Path $binPath -Algorithm SHA256).Hash.ToLower()
    if ($actual -ne $expected) {
        Remove-Item $binPath -Force
        Write-Error "Checksum mismatch! Expected: $expected, Got: $actual"
        exit 1
    }
    Write-Host "Checksum verified: $actual" -ForegroundColor Green
}

# Migrate legacy config if present and new config doesn't exist
$configFile = Join-Path $configDir "config.toml"
$legacyConfigFile = Join-Path $legacyConfigDir "config.toml"
if ((Test-Path $legacyConfigFile) -and -not (Test-Path $configFile)) {
    New-Item -ItemType Directory -Force -Path $configDir | Out-Null
    Copy-Item $legacyConfigFile $configFile -Force
    Write-Host "Migrated config from $legacyConfigDir"
}

# Always prompt for token and rewrite config
New-Item -ItemType Directory -Force -Path $configDir | Out-Null
$token = ""
while ([string]::IsNullOrWhiteSpace($token)) {
    $token = Read-Host "Enter your Pling API token"
    $token = $token -replace '["\\\r\n\s]', ''
    if ([string]::IsNullOrWhiteSpace($token)) {
        Write-Host "Token cannot be empty." -ForegroundColor Yellow
    }
}
@"
token = "$token"
api_url = "https://agent.plingpush.com"
metrics_interval = 60
allow_remote_commands = true
webui_port = 9876
"@ | Set-Content $configFile
Write-Host "Config written to $configFile"

# Clean up legacy paths
if (Test-Path $legacyConfigDir) { Remove-Item $legacyConfigDir -Recurse -Force -ErrorAction SilentlyContinue }
if (Test-Path $legacyInstallDir) { Remove-Item $legacyInstallDir -Recurse -Force -ErrorAction SilentlyContinue }

# Install as Windows service
$existing = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
if ($existing) {
    Stop-Service -Name $serviceName -Force -ErrorAction SilentlyContinue
    sc.exe delete $serviceName | Out-Null
    Start-Sleep -Seconds 2
}

Write-Host "Creating Windows service..."
sc.exe create $serviceName binPath= "`"$binPath`" serve" start= auto DisplayName= "Pling" | Out-Null
sc.exe description $serviceName "Pling - 24/7 server monitoring and scheduled commands" | Out-Null
sc.exe failure $serviceName reset= 60 actions= restart/10000/restart/30000/restart/60000 | Out-Null
Start-Service -Name $serviceName

$ip = (Get-NetIPAddress -AddressFamily IPv4 | Where-Object { $_.InterfaceAlias -notmatch "Loopback" -and $_.PrefixOrigin -ne "WellKnown" } | Select-Object -First 1).IPAddress
Write-Host ""
Write-Host "Done! Pling is running." -ForegroundColor Green
Write-Host "Try: pling status" -ForegroundColor Cyan
Write-Host "Agent settings: http://${ip}:9876" -ForegroundColor Cyan
