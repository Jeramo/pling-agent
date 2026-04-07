# Pling Agent

A lightweight agent that runs on your server to report system metrics, relay terminal sharing sessions, and execute scheduled commands, all controlled from the [Pling](https://plingpush.com) iOS app.

## Features

- **System metrics** — CPU, memory, disk, network, and load average reported to Pling
- **Terminal sharing** — relay shared SSH sessions through the agent for persistent connections
- **Scheduled commands** — run commands on a cron schedule, triggered from the app
- **Auto-updates** — the agent updates itself when new versions are released
- **Web UI** — local settings dashboard at `http://localhost:9876`

## Install

### Linux / macOS

```sh
curl -sSL https://raw.githubusercontent.com/Jeramo/pling-agent/main/install.sh | sh
```

The installer downloads the latest release, prompts for your API token, and sets up a systemd service (Linux) or LaunchAgent (macOS).

### Windows

```powershell
irm https://raw.githubusercontent.com/Jeramo/pling-agent/main/install.ps1 | iex
```

### Manual

Download the binary for your platform from the [latest release](https://github.com/Jeramo/pling-agent/releases/latest), then:

```sh
chmod +x pling-agent-*
sudo mv pling-agent-* /usr/local/bin/pling-agent
sudo mkdir -p /etc/pling-agent
sudo nano /etc/pling-agent/config.toml
```

Config file (`/etc/pling-agent/config.toml`):

```toml
api_url = "https://agent.plingpush.com"
token = "YOUR_TOKEN_HERE"
metrics_interval = 60
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
go build -o pling-agent ./cmd/pling-agent
```

## Uninstall

### Linux

```sh
sudo systemctl stop pling-agent
sudo systemctl disable pling-agent
sudo rm /etc/systemd/system/pling-agent.service
sudo rm /usr/local/bin/pling-agent
sudo rm -rf /etc/pling-agent
```

### macOS

```sh
launchctl bootout gui/$(id -u) ~/Library/LaunchAgents/com.jeramo.pling-agent.plist
rm ~/Library/LaunchAgents/com.jeramo.pling-agent.plist
sudo rm /usr/local/bin/pling-agent
sudo rm -rf /etc/pling-agent
```

## License

Proprietary. Copyright (c) 2026 Jean-Robert Nino.
