# Pling

A lightweight agent + CLI that connects your server or workstation to the [Pling](https://plingpush.com) app. Reports system metrics, relays shared terminal sessions, runs scheduled commands, and exposes a small web UI for local config.

## Install

### Linux / macOS

```sh
curl -sSL https://raw.githubusercontent.com/Jeramo/pling-agent/main/install.sh | sh
```

The installer downloads the latest release, prompts for your API token, and sets up a systemd service (Linux) or LaunchAgent (macOS). It also migrates from any earlier `pling-agent` install.

### macOS via Homebrew

```sh
brew install jeramo/pling/pling
brew services start pling
```

### Windows

```powershell
irm https://raw.githubusercontent.com/Jeramo/pling-agent/main/install.ps1 | iex
```

### Manual

```sh
curl -L -o pling https://github.com/Jeramo/pling-agent/releases/latest/download/pling-linux-amd64
chmod +x pling
sudo mv pling /usr/local/bin/pling
sudo mkdir -p /etc/pling
sudo nano /etc/pling/config.toml
```

Config file (`/etc/pling/config.toml`):

```toml
api_url = "https://agent.plingpush.com"
token = "YOUR_TOKEN_HERE"
metrics_interval = 60
```

Then run `pling serve`, or install it as a service.

## CLI

```
pling serve              run the agent (foreground daemon; what the service runs)
pling status             service state, version, config path
pling logs [-f]          recent agent logs (-f to follow)
pling start | stop | restart
pling set-token <token>  update token and restart service
pling config [edit]      print config path or open in $EDITOR
pling open               open the local web UI
pling uninstall          remove agent, config, and service
pling version
```

## Supported platforms

| Platform | Architecture |
|----------|-------------|
| Linux | amd64, arm64 |
| macOS | amd64, arm64 |
| FreeBSD | amd64 |
| Windows | amd64 |

macOS binaries are signed with Developer ID and notarized by Apple.

## Build from source

```sh
go build -o pling ./cmd/pling-agent
```

## Uninstall

```sh
pling uninstall
```

## License

Proprietary. Copyright (c) 2025 Jean-Robert Nino.
