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

# Verify SHA-256 checksum
CHECKSUM_URL="${LATEST}.sha256"
EXPECTED=$(curl -sL "$CHECKSUM_URL" | awk '{print $1}')
if [ -n "$EXPECTED" ]; then
    if command -v sha256sum > /dev/null 2>&1; then
        ACTUAL=$(sha256sum "$TMPBIN" | awk '{print $1}')
    elif command -v shasum > /dev/null 2>&1; then
        ACTUAL=$(shasum -a 256 "$TMPBIN" | awk '{print $1}')
    else
        echo "Warning: cannot verify checksum (sha256sum/shasum not found)"
        ACTUAL="$EXPECTED"
    fi
    if [ "$ACTUAL" != "$EXPECTED" ]; then
        echo "Checksum mismatch! Expected: ${EXPECTED}, Got: ${ACTUAL}"
        rm -f "$TMPBIN"
        exit 1
    fi
    echo "Checksum verified: ${ACTUAL}"
else
    echo "Warning: no checksum file available, skipping verification"
fi

sudo mv "$TMPBIN" "$INSTALL_DIR/pling-agent"
# Remove macOS quarantine flag to prevent Gatekeeper "unidentified developer" warning
if [ "$OS" = "darwin" ]; then
    sudo xattr -d com.apple.quarantine "$INSTALL_DIR/pling-agent" 2>/dev/null || true
fi
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
    # Write token separately — strip any characters that could break TOML quoting
    CLEAN_TOKEN=$(printf '%s' "$TOKEN" | tr -d '"\\' | tr -d '\n')
    printf 'token = "%s"\n' "$CLEAN_TOKEN" >> "$TMPCONF"
    sudo mv "$TMPCONF" "${CONFIG_DIR}/config.toml"
    sudo chmod 600 "${CONFIG_DIR}/config.toml"
    # On macOS the launchd agent runs as the current user, so chown to them.
    # On Linux the systemd service runs as root, so leave ownership as root.
    if [ "$OS" = "darwin" ]; then
        sudo chown "$(id -u):$(id -g)" "${CONFIG_DIR}/config.toml"
        sudo chown "$(id -u):$(id -g)" "${CONFIG_DIR}"
    fi
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
    sudo systemctl restart pling-agent
    echo "Systemd service started"
    IP=$(hostname -I 2>/dev/null | awk '{print $1}')
    [ -n "$IP" ] && echo "Agent settings: http://${IP}:9876"

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

    # Install menu bar tray icon
    TRAY_URL=$(curl -sL "https://api.github.com/repos/${REPO}/releases/latest" | grep "browser_download_url.*pling-tray-darwin-${ARCH}" | head -1 | cut -d '"' -f 4)
    if [ -n "$TRAY_URL" ]; then
        TMPTRAY=$(mktemp /tmp/pling-tray.XXXXXX)
        curl -sL "$TRAY_URL" -o "$TMPTRAY"
        chmod +x "$TMPTRAY"
        sudo mv "$TMPTRAY" "$INSTALL_DIR/pling-tray"
        sudo xattr -d com.apple.quarantine "$INSTALL_DIR/pling-tray" 2>/dev/null || true

        TRAY_PLIST="$HOME/Library/LaunchAgents/com.jeramo.pling-tray.plist"
        cat > "$TRAY_PLIST" <<TEOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>com.jeramo.pling-tray</string>
    <key>ProgramArguments</key><array><string>${INSTALL_DIR}/pling-tray</string></array>
    <key>RunAtLoad</key><true/>
    <key>KeepAlive</key><true/>
    <key>LSUIElement</key><true/>
</dict>
</plist>
TEOF
        launchctl bootout gui/$(id -u) "$TRAY_PLIST" 2>/dev/null || true
        launchctl bootstrap gui/$(id -u) "$TRAY_PLIST"
        echo "Menu bar icon installed"
    fi
fi

echo "Done! pling-agent is running."
