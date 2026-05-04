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
# Remove macOS quarantine flag to prevent Gatekeeper warning
if [ "$OS" = "darwin" ]; then
    sudo xattr -d com.apple.quarantine "$INSTALL_DIR/pling-agent" 2>/dev/null || true
fi
echo "Installed to ${INSTALL_DIR}/pling-agent"

sudo mkdir -p "$CONFIG_DIR"
sudo chmod 700 "$CONFIG_DIR"
if [ ! -t 0 ] && [ ! -r /dev/tty ]; then
    echo "Error: no TTY available to read API token. Run installer in an interactive shell." >&2
    exit 1
fi
TOKEN=""
while [ -z "$TOKEN" ]; do
    printf "Enter your Pling API token: "
    read -r TOKEN < /dev/tty
    TOKEN=$(printf '%s' "$TOKEN" | tr -d '"\\' | tr -d '\n' | tr -d '[:space:]')
    if [ -z "$TOKEN" ]; then
        echo "Token cannot be empty. Try again." >&2
    fi
done
TMPCONF=$(mktemp /tmp/pling-config.XXXXXX)
chmod 600 "$TMPCONF"
cat > "$TMPCONF" <<'EOF'
api_url = "https://agent.plingpush.com"
metrics_interval = 60
EOF
printf 'token = "%s"\n' "$TOKEN" >> "$TMPCONF"
sudo mv "$TMPCONF" "${CONFIG_DIR}/config.toml"
sudo chmod 600 "${CONFIG_DIR}/config.toml"
if [ "$OS" = "darwin" ]; then
    sudo chown "$(id -u):$(id -g)" "${CONFIG_DIR}/config.toml"
    sudo chown "$(id -u):$(id -g)" "${CONFIG_DIR}"
fi
echo "Config written to ${CONFIG_DIR}/config.toml"

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
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
    </dict>
</dict>
</plist>
EOF
    launchctl bootout gui/$(id -u) "$PLIST" 2>/dev/null || true
    launchctl bootstrap gui/$(id -u) "$PLIST"
    echo "LaunchAgent started"
fi

echo "Done! pling-agent is running."

# Open the web UI so the user can verify/configure the agent
sleep 2
if command -v xdg-open > /dev/null 2>&1; then
    xdg-open "http://localhost:9876" 2>/dev/null &
elif command -v open > /dev/null 2>&1; then
    open "http://localhost:9876"
fi
