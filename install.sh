#!/bin/sh
set -e

REPO="jeramo/pling-agent"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/pling-agent"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

echo "Detected: ${OS}/${ARCH}"

LATEST=$(curl -sL "https://api.github.com/repos/${REPO}/releases/latest" | grep "browser_download_url.*${OS}-${ARCH}" | head -1 | cut -d '"' -f 4)
if [ -z "$LATEST" ]; then
    echo "No release found for ${OS}/${ARCH}"
    exit 1
fi

TMPBIN=$(mktemp /tmp/pling-agent.XXXXXX)
echo "Downloading ${LATEST}..."
curl -sL "$LATEST" -o "$TMPBIN"
chmod +x "$TMPBIN"
sudo mv "$TMPBIN" "$INSTALL_DIR/pling-agent"
echo "Installed to ${INSTALL_DIR}/pling-agent"

if [ ! -f "${CONFIG_DIR}/config.toml" ]; then
    sudo mkdir -p "$CONFIG_DIR"
    sudo chmod 700 "$CONFIG_DIR"
    printf "Enter your Pling API token: "
    read -r TOKEN < /dev/tty
    TMPCONF=$(mktemp /tmp/pling-config.XXXXXX)
    chmod 600 "$TMPCONF"
    cat > "$TMPCONF" <<'EOF'
api_url = "https://agent.plingpush.com"
metrics_interval = 60
EOF
    # Write token separately to avoid shell injection from user input
    printf 'token = "%s"\n' "$TOKEN" >> "$TMPCONF"
    sudo mv "$TMPCONF" "${CONFIG_DIR}/config.toml"
    sudo chmod 600 "${CONFIG_DIR}/config.toml"
    sudo chown "$(id -u):$(id -g)" "${CONFIG_DIR}/config.toml"
    echo "Config written to ${CONFIG_DIR}/config.toml"
fi

if [ "$OS" = "linux" ] && command -v systemctl > /dev/null 2>&1; then
    sudo tee /etc/systemd/system/pling-agent.service > /dev/null <<EOF
[Unit]
Description=Pling Agent
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=${INSTALL_DIR}/pling-agent
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF
    sudo systemctl daemon-reload
    sudo systemctl enable pling-agent
    sudo systemctl start pling-agent
    echo "Systemd service started"

elif [ "$OS" = "darwin" ]; then
    PLIST="$HOME/Library/LaunchAgents/com.jeramo.pling-agent.plist"
    cat > "$PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>com.jeramo.pling-agent</string>
    <key>ProgramArguments</key><array><string>${INSTALL_DIR}/pling-agent</string></array>
    <key>RunAtLoad</key><true/>
    <key>KeepAlive</key><true/>
</dict>
</plist>
EOF
    launchctl bootout gui/$(id -u) "$PLIST" 2>/dev/null || true
    launchctl bootstrap gui/$(id -u) "$PLIST"
    echo "LaunchAgent started"
fi

echo "Done! pling-agent is running."
