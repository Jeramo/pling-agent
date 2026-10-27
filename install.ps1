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

# Trusted principals for the config/token store: LocalSystem (the service)
# and Administrators, by SID so this works on every Windows language.
$SID_SYSTEM = "S-1-5-18"
$SID_ADMINS = "S-1-5-32-544"
# System tools by absolute path: this script runs elevated, so it must not
# resolve icacls / sc from a PATH an attacker could influence.
$icacls = Join-Path $env:SystemRoot "System32\icacls.exe"
$scExe = Join-Path $env:SystemRoot "System32\sc.exe"

function Invoke-Icacls {
    param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Args)
    $out = & $icacls @Args 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "icacls $($Args -join ' ') failed ($LASTEXITCODE): $out"
    }
}

# Secure-ConfigDir makes $configDir safe for the SYSTEM service to trust
# BEFORE any token, config or key is written into it: the token decides whose
# commands that service runs as SYSTEM, so a standard user must not be able to
# read it or rewrite the config. A standard user can pre-create ProgramData
# subfolders (CREATE_OWNER), so a dir not already owned by SYSTEM/Admins is
# moved aside unread; then owner -> Administrators, inheritance removed,
# SYSTEM + Administrators full control only, children reset to inherit. The
# ACL is verified before the function returns.
function Secure-ConfigDir {
    if (Test-Path $configDir) {
        $acl = Get-Acl -LiteralPath $configDir
        $ownerSid = $acl.GetOwner([System.Security.Principal.SecurityIdentifier]).Value
        $isLink = (Get-Item -LiteralPath $configDir -Force).Attributes -band [IO.FileAttributes]::ReparsePoint
        if ($isLink -or ($ownerSid -ne $SID_SYSTEM -and $ownerSid -ne $SID_ADMINS)) {
            $aside = "$configDir-untrusted-" + (Get-Date -Format "yyyyMMdd-HHmmss")
            Write-Host "Config dir was a link or not owned by SYSTEM/Administrators; moving it aside to $aside" -ForegroundColor Yellow
            Rename-Item -LiteralPath $configDir -NewName (Split-Path $aside -Leaf) -Force
        }
    }
    New-Item -ItemType Directory -Force -Path $configDir | Out-Null
    Invoke-Icacls $configDir /setowner "*$SID_ADMINS" /T /C /Q
    Invoke-Icacls $configDir /inheritance:r /grant:r "*${SID_SYSTEM}:(OI)(CI)F" "*${SID_ADMINS}:(OI)(CI)F" /C /Q
    if ((Get-ChildItem -LiteralPath $configDir -Force | Measure-Object).Count -gt 0) {
        Invoke-Icacls (Join-Path $configDir "*") /reset /T /C /Q
    }
    # Verify: Administrators or SYSTEM owns it, and no other principal has an
    # allow entry (inheritance is off, so the DACL is exactly what we set).
    $acl = Get-Acl -LiteralPath $configDir
    $ownerSid = $acl.GetOwner([System.Security.Principal.SecurityIdentifier]).Value
    if ($ownerSid -ne $SID_SYSTEM -and $ownerSid -ne $SID_ADMINS) {
        throw "Could not take ownership of $configDir (owner $ownerSid)"
    }
    foreach ($ace in $acl.Access) {
        if ($ace.AccessControlType -ne [System.Security.AccessControl.AccessControlType]::Allow) { continue }
        $sid = $ace.IdentityReference.Translate([System.Security.Principal.SecurityIdentifier]).Value
        if ($sid -ne $SID_SYSTEM -and $sid -ne $SID_ADMINS) {
            throw "$configDir is still reachable by $sid after locking it down"
        }
    }
}

# Detect architecture
$cpuArch = $env:PROCESSOR_ARCHITECTURE
$arch = if ($cpuArch -eq "ARM64") { "arm64" } elseif ([Environment]::Is64BitOperatingSystem) { "amd64" } else { Write-Error "32-bit Windows not supported"; exit 1 }

Write-Host "Detected: windows/$arch" -ForegroundColor Cyan

# Stop legacy service if present
$legacy = Get-Service -Name $legacyService -ErrorAction SilentlyContinue
if ($legacy) {
    Write-Host "Stopping legacy PlingAgent service..."
    Stop-Service -Name $legacyService -Force -ErrorAction SilentlyContinue
    & $scExe delete $legacyService | Out-Null
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
Invoke-WebRequest -UseBasicParsing -Uri $asset.browser_download_url -OutFile $binPath

# Verify SHA-256 checksum if published
$checksumAsset = $release.assets | Where-Object { $_.name -eq "pling-windows-$arch.exe.sha256" } | Select-Object -First 1
if ($checksumAsset) {
    # -UseBasicParsing: without it PowerShell 5 parses with the IE engine and
    # throws in non-interactive sessions (SSH). GitHub serves the asset as
    # octet-stream, so Content can arrive as bytes.
    $checksumContent = (Invoke-WebRequest -UseBasicParsing -Uri $checksumAsset.browser_download_url).Content
    if ($checksumContent -is [byte[]]) { $checksumContent = [System.Text.Encoding]::ASCII.GetString($checksumContent) }
    $expected = ($checksumContent -split '\s')[0].Trim().ToLower()
    $actual = (Get-FileHash -Path $binPath -Algorithm SHA256).Hash.ToLower()
    if ($actual -ne $expected) {
        Remove-Item $binPath -Force
        Write-Error "Checksum mismatch! Expected: $expected, Got: $actual"
        exit 1
    }
    Write-Host "Checksum verified: $actual" -ForegroundColor Green
}

# Lock the config dir down FIRST, before any token/config/key is written: a
# token in a Users-readable file is a standard-user -> SYSTEM escalation.
$configFile = Join-Path $configDir "config.toml"
$legacyConfigFile = Join-Path $legacyConfigDir "config.toml"
Secure-ConfigDir
Write-Host "Config directory locked to SYSTEM and Administrators" -ForegroundColor Green

# Prompt for the token and write a FRESH config file (delete any existing one
# first, so a read handle on the old file can't see the new token, and the new
# file inherits only the locked ACL set above).
$token = ""
while ([string]::IsNullOrWhiteSpace($token)) {
    $token = Read-Host "Enter your Pling API token"
    $token = $token -replace '["\\\r\n\s]', ''
    if ([string]::IsNullOrWhiteSpace($token)) {
        Write-Host "Token cannot be empty." -ForegroundColor Yellow
    }
}
Remove-Item -LiteralPath $configFile -Force -ErrorAction SilentlyContinue
@"
token = "$token"
api_url = "https://agent.plingpush.com"
metrics_interval = 60
allow_remote_commands = true
webui_port = 9876
"@ | Set-Content -LiteralPath $configFile -Encoding ASCII
# The new file inherits SYSTEM+Administrators full control from the dir;
# reset it to be sure it carries no explicit, pre-existing entry.
Invoke-Icacls $configFile /reset /C /Q
Write-Host "Config written to $configFile"
if (Test-Path $legacyConfigFile) {
    Write-Host "A legacy config at $legacyConfigFile was not migrated; its token (if any) may have been readable by other users -- rotate it." -ForegroundColor Yellow
}

# Clean up legacy paths
if (Test-Path $legacyConfigDir) { Remove-Item $legacyConfigDir -Recurse -Force -ErrorAction SilentlyContinue }
if (Test-Path $legacyInstallDir) { Remove-Item $legacyInstallDir -Recurse -Force -ErrorAction SilentlyContinue }

# Install as Windows service
$existing = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
if ($existing) {
    Stop-Service -Name $serviceName -Force -ErrorAction SilentlyContinue
    & $scExe delete $serviceName | Out-Null
    Start-Sleep -Seconds 2
}

Write-Host "Creating Windows service..."
# New-Service, not `sc.exe create`: Windows PowerShell 5.1 passes an argument
# that starts with an embedded quote through unescaped, so sc.exe split the
# quoted binPath and printed its usage instead of creating the service.
New-Service -Name $serviceName -BinaryPathName "`"$binPath`" serve" -DisplayName "Pling" `
    -StartupType Automatic -Description "Pling - 24/7 server monitoring and scheduled commands" | Out-Null
$failureOut = & $scExe failure $serviceName reset= 60 actions= restart/10000/restart/30000/restart/60000 2>&1
if ($LASTEXITCODE -ne 0) { Write-Host "Could not set the service restart policy: $failureOut" -ForegroundColor Yellow }
Start-Service -Name $serviceName

# Add install dir to system PATH so `pling` works from any shell
$systemPath = [Environment]::GetEnvironmentVariable("Path", "Machine")
$pathEntries = $systemPath -split ';' | Where-Object { $_ -ne '' }
if ($pathEntries -notcontains $installDir) {
    [Environment]::SetEnvironmentVariable("Path", "$systemPath;$installDir", "Machine")
    Write-Host "Added $installDir to system PATH (open a new shell to use it)"
}
# Also prepend to current session's PATH so `pling status` works right after install
if (-not ($env:Path -split ';' -contains $installDir)) {
    $env:Path = "$installDir;$env:Path"
}

# Remove legacy install dir from PATH if present
$legacyOnPath = $pathEntries -contains $legacyInstallDir
if ($legacyOnPath) {
    $cleaned = ($pathEntries | Where-Object { $_ -ne $legacyInstallDir }) -join ';'
    [Environment]::SetEnvironmentVariable("Path", $cleaned, "Machine")
}

# Screen (remote desktop from the Pling app) streams the desktop with ffmpeg.
# Only machines with a desktop need it; a failure here never fails the install.
$hasDesktop = Test-Path (Join-Path $env:SystemRoot "explorer.exe")
if ($hasDesktop) {
    $ffmpegFound = [bool](Get-Command ffmpeg -ErrorAction SilentlyContinue)
    if (-not $ffmpegFound) {
        $ffmpegPaths = @(
            "$env:LOCALAPPDATA\Microsoft\WinGet\Links\ffmpeg.exe",
            "$env:ProgramFiles\WinGet\Links\ffmpeg.exe",
            "$env:ProgramFiles\ffmpeg\bin\ffmpeg.exe",
            "$env:ProgramData\chocolatey\bin\ffmpeg.exe",
            "$env:USERPROFILE\scoop\shims\ffmpeg.exe",
            "$env:ProgramData\scoop\shims\ffmpeg.exe"
        )
        $ffmpegFound = [bool]($ffmpegPaths | Where-Object { Test-Path $_ } | Select-Object -First 1)
    }
    if (-not $ffmpegFound) {
        $wingetCmd = "winget install --id Gyan.FFmpeg -e --scope machine --accept-package-agreements --accept-source-agreements"
        if (Get-Command winget -ErrorAction SilentlyContinue) {
            Write-Host "Installing ffmpeg for Pling Screen (winget Gyan.FFmpeg)..."
            try {
                # Machine scope puts it in Program Files\WinGet\Links, where the
                # Screen helper finds it for whoever signs in; fall back to the
                # default (this user's) scope if the package refuses.
                winget install --id Gyan.FFmpeg -e --scope machine --silent --disable-interactivity --accept-package-agreements --accept-source-agreements
                if ($LASTEXITCODE -ne 0) {
                    winget install --id Gyan.FFmpeg -e --silent --disable-interactivity --accept-package-agreements --accept-source-agreements
                }
                if ($LASTEXITCODE -ne 0) { throw "winget exit code $LASTEXITCODE" }
            } catch {
                Write-Host "Could not install ffmpeg ($_). Screen needs it; install it with:" -ForegroundColor Yellow
                Write-Host "  $wingetCmd" -ForegroundColor Yellow
            }
        } else {
            Write-Host "Pling Screen needs ffmpeg. Install it with:" -ForegroundColor Yellow
            Write-Host "  $wingetCmd" -ForegroundColor Yellow
            Write-Host "  (or download it from https://www.gyan.dev/ffmpeg/builds/)" -ForegroundColor Yellow
        }
    }
}

$ip = (Get-NetIPAddress -AddressFamily IPv4 | Where-Object { $_.InterfaceAlias -notmatch "Loopback" -and $_.PrefixOrigin -ne "WellKnown" } | Select-Object -First 1).IPAddress
Write-Host ""
Write-Host "Done! Pling is running." -ForegroundColor Green
Write-Host "Try: pling status" -ForegroundColor Cyan
Write-Host "Agent settings: http://${ip}:9876" -ForegroundColor Cyan
