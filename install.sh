#!/bin/sh
set -e

REPO="jeramo/pling-agent"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/pling"
LEGACY_CONFIG_DIR="/etc/pling-agent"

# --system installs a root LaunchDaemon on macOS so the agent boots before login
# (headless Macs). Trade-off: a daemon has no GUI session, so Screen (video beta)
# and remote mouse/keyboard are unavailable. Default is a per-user LaunchAgent
# that loads on GUI login and keeps Screen working. No effect on Linux/Windows,
# which already start before login.
SYSTEM=0
for arg in "$@"; do
    case "$arg" in
        --system) SYSTEM=1 ;;
    esac
done

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

echo "Detected: ${OS}/${ARCH}"

# Deterministic asset URL — scraping the release JSON for a matching name could
# pick the .sha256 file instead, depending on asset order.
DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/pling-${OS}-${ARCH}"

# Stop legacy service if present so we can replace its files
if [ "$OS" = "linux" ] && command -v systemctl > /dev/null 2>&1; then
    sudo systemctl stop pling-agent 2>/dev/null || true
    sudo systemctl disable pling-agent 2>/dev/null || true
elif [ "$OS" = "darwin" ]; then
    LEGACY_PLIST="$HOME/Library/LaunchAgents/com.jeramo.pling-agent.plist"
    if [ -f "$LEGACY_PLIST" ]; then
        launchctl bootout "gui/$(id -u)" "$LEGACY_PLIST" 2>/dev/null || true
        rm -f "$LEGACY_PLIST"
    fi
fi

TMPBIN=$(mktemp /tmp/pling.XXXXXX)
echo "Downloading ${DOWNLOAD_URL}..."
if ! curl -fsSL "$DOWNLOAD_URL" -o "$TMPBIN"; then
    echo "Download failed (no release for ${OS}/${ARCH}?)" >&2
    rm -f "$TMPBIN"
    exit 1
fi

# Verify against the published checksum. Its content is "<hex>  <dist-path>";
# compare the hash field only — the recorded path is the release build's.
TMPSUM=$(mktemp /tmp/pling-sha.XXXXXX)
if ! curl -fsSL "${DOWNLOAD_URL}.sha256" -o "$TMPSUM"; then
    echo "Failed to download checksum ${DOWNLOAD_URL}.sha256" >&2
    rm -f "$TMPBIN" "$TMPSUM"
    exit 1
fi
EXPECTED=$(awk '{print $1}' "$TMPSUM")
rm -f "$TMPSUM"
if [ "$OS" = "darwin" ]; then
    ACTUAL=$(shasum -a 256 "$TMPBIN" | awk '{print $1}')
else
    ACTUAL=$(sha256sum "$TMPBIN" | awk '{print $1}')
fi
if [ -z "$EXPECTED" ] || [ "$ACTUAL" != "$EXPECTED" ]; then
    echo "Checksum mismatch for pling-${OS}-${ARCH}: expected '${EXPECTED}', got '${ACTUAL}'" >&2
    rm -f "$TMPBIN"
    exit 1
fi
echo "Checksum verified"
chmod +x "$TMPBIN"

sudo mv "$TMPBIN" "$INSTALL_DIR/pling"
# Drop a legacy symlink so old service units / docs keep working until uninstall
sudo ln -sf "$INSTALL_DIR/pling" "$INSTALL_DIR/pling-agent"
if [ "$OS" = "darwin" ]; then
    sudo xattr -d com.apple.quarantine "$INSTALL_DIR/pling" 2>/dev/null || true
fi
echo "Installed to ${INSTALL_DIR}/pling"

# Migrate legacy config if present and new config doesn't exist
if [ -f "${LEGACY_CONFIG_DIR}/config.toml" ] && [ ! -f "${CONFIG_DIR}/config.toml" ]; then
    sudo mkdir -p "$CONFIG_DIR"
    sudo chmod 700 "$CONFIG_DIR"
    sudo cp "${LEGACY_CONFIG_DIR}/config.toml" "${CONFIG_DIR}/config.toml"
    sudo chmod 600 "${CONFIG_DIR}/config.toml"
    echo "Migrated config from ${LEGACY_CONFIG_DIR}"
fi

sudo mkdir -p "$CONFIG_DIR"
sudo chmod 700 "$CONFIG_DIR"
CONFIG_FILE="${CONFIG_DIR}/config.toml"
if [ -f "$CONFIG_FILE" ]; then
    # Reinstall/upgrade: keep the existing config (webui_port, hostname_override,
    # allow_remote_commands, ...); only replace the token if a new one is given.
    echo "Existing config found at ${CONFIG_FILE} - keeping it."
    TOKEN=""
    if [ -t 0 ] || [ -r /dev/tty ]; then
        printf "Enter your Pling API token (leave empty to keep the current one): "
        read -r TOKEN < /dev/tty
        TOKEN=$(printf '%s' "$TOKEN" | tr -d '"\\' | tr -d '\n' | tr -d '[:space:]')
    fi
    if [ -n "$TOKEN" ]; then
        if sudo grep -q '^token[[:space:]]*=' "$CONFIG_FILE"; then
            # -i.bak (attached suffix) is the portable in-place form for BSD + GNU sed
            sudo sed -i.bak "s|^token[[:space:]]*=.*|token = \"${TOKEN}\"|" "$CONFIG_FILE"
            sudo rm -f "${CONFIG_FILE}.bak"
        else
            printf 'token = "%s"\n' "$TOKEN" | sudo tee -a "$CONFIG_FILE" > /dev/null
        fi
        echo "Token updated in ${CONFIG_FILE}"
    fi
else
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
    sudo mv "$TMPCONF" "$CONFIG_FILE"
    echo "Config written to ${CONFIG_FILE}"
fi
sudo chmod 600 "$CONFIG_FILE"
# LaunchAgent runs in the user's context and needs to read the config; a root
# LaunchDaemon (--system) reads it as root, so leave it root-owned there.
if [ "$OS" = "darwin" ] && [ "$SYSTEM" != "1" ]; then
    sudo chown "$(id -u):$(id -g)" "$CONFIG_FILE"
    sudo chown "$(id -u):$(id -g)" "$CONFIG_DIR"
fi

# Remove legacy config dir now that we've migrated
if [ -d "$LEGACY_CONFIG_DIR" ]; then
    sudo rm -rf "$LEGACY_CONFIG_DIR" 2>/dev/null || true
fi

if [ "$OS" = "linux" ] && command -v systemctl > /dev/null 2>&1; then
    sudo rm -f /etc/systemd/system/pling-agent.service
    sudo tee /etc/systemd/system/pling.service > /dev/null <<EOF
[Unit]
Description=Pling agent
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=${INSTALL_DIR}/pling serve
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF
    sudo systemctl daemon-reload
    sudo systemctl enable pling
    sudo systemctl restart pling
    echo "Systemd service started"
    IP=$(hostname -I 2>/dev/null | awk '{print $1}')
    [ -n "$IP" ] && echo "Agent settings: http://${IP}:9876"

    # Screen (remote desktop from the app) streams an X11 desktop through
    # ffmpeg. Only on machines with a desktop; a headless server is left alone,
    # and nothing here may fail the install.
    if { systemctl is-active --quiet display-manager 2>/dev/null || [ -n "${DISPLAY}${WAYLAND_DISPLAY}" ]; } \
        && ! command -v ffmpeg > /dev/null 2>&1; then
        . /etc/os-release 2>/dev/null || true
        case " ${ID:-} ${ID_LIKE:-} " in
            *" debian "*|*" ubuntu "*) FFMPEG_CMD="sudo apt-get update -qq && sudo apt-get install -y ffmpeg" ;;
            *" fedora "*) FFMPEG_CMD="sudo dnf install -y ffmpeg-free" ;;
            *" arch "*) FFMPEG_CMD="sudo pacman -S --noconfirm ffmpeg" ;;
            *" opensuse"*|*" suse "*) FFMPEG_CMD="sudo zypper --non-interactive install ffmpeg" ;;
            *" alpine "*) FFMPEG_CMD="sudo apk add ffmpeg" ;;
            *) FFMPEG_CMD="" ;;
        esac
        if [ -n "$FFMPEG_CMD" ]; then
            echo "Installing ffmpeg for Screen: $FFMPEG_CMD"
            sh -c "$FFMPEG_CMD" > /dev/null 2>&1 || echo "Could not install ffmpeg. Screen needs it: $FFMPEG_CMD"
        else
            echo "Screen needs ffmpeg: install it with your distribution's package manager."
        fi
    fi

    # On Wayland desktops (GNOME, KDE, ...) Screen reads the screen from
    # PipeWire and encodes it with GStreamer. Same rules: only with a Wayland
    # session, and never failing the install.
    WAYLAND_DESKTOP=0
    if [ "${XDG_SESSION_TYPE:-}" = "wayland" ] || [ -n "${WAYLAND_DISPLAY:-}" ]; then
        WAYLAND_DESKTOP=1
    elif command -v loginctl > /dev/null 2>&1; then
        for S in $(loginctl list-sessions --no-legend 2>/dev/null | awk '{print $1}'); do
            if [ "$(loginctl show-session "$S" -p Type --value 2>/dev/null)" = "wayland" ]; then
                WAYLAND_DESKTOP=1
            fi
        done
    fi
    if [ "$WAYLAND_DESKTOP" = "1" ] && ! { command -v gst-launch-1.0 > /dev/null 2>&1 \
        && gst-inspect-1.0 pipewiresrc > /dev/null 2>&1 && gst-inspect-1.0 h264parse > /dev/null 2>&1 \
        && { gst-inspect-1.0 x264enc > /dev/null 2>&1 || gst-inspect-1.0 openh264enc > /dev/null 2>&1; }; }; then
        . /etc/os-release 2>/dev/null || true
        case " ${ID:-} ${ID_LIKE:-} " in
            *" debian "*|*" ubuntu "*) GST_CMD="sudo apt-get update -qq && sudo apt-get install -y gstreamer1.0-tools gstreamer1.0-pipewire gstreamer1.0-plugins-base gstreamer1.0-plugins-good gstreamer1.0-plugins-bad gstreamer1.0-plugins-ugly" ;;
            *" fedora "*) GST_CMD="sudo dnf install -y gstreamer1 pipewire-gstreamer gstreamer1-plugins-base gstreamer1-plugins-good gstreamer1-plugins-bad-free gstreamer1-plugin-openh264" ;;
            *" arch "*) GST_CMD="sudo pacman -S --noconfirm gstreamer gst-plugin-pipewire gst-plugins-base gst-plugins-good gst-plugins-bad gst-plugins-ugly" ;;
            *" opensuse"*|*" suse "*) GST_CMD="sudo zypper --non-interactive install gstreamer-utils gstreamer-plugin-pipewire gstreamer-plugins-base gstreamer-plugins-good gstreamer-plugins-bad gstreamer-plugin-openh264" ;;
            *" alpine "*) GST_CMD="sudo apk add gstreamer-tools gst-plugin-pipewire gst-plugins-base gst-plugins-good gst-plugins-bad gst-plugins-ugly" ;;
            *) GST_CMD="" ;;
        esac
        if [ -n "$GST_CMD" ]; then
            echo "Installing the PipeWire/GStreamer pieces Screen needs on Wayland: $GST_CMD"
            sh -c "$GST_CMD" > /dev/null 2>&1 || echo "Could not install them. Screen on Wayland needs: $GST_CMD"
        else
            echo "Screen on Wayland needs GStreamer with the PipeWire plugin, h264parse and an H.264 encoder (x264 or openh264)."
        fi
    fi

elif [ "$OS" = "darwin" ] && [ "$SYSTEM" = "1" ]; then
    # Boot-before-login: install a root LaunchDaemon. Remove any per-user
    # LaunchAgent first so two `pling serve` processes don't fight over port 9876.
    AGENT_PLIST="$HOME/Library/LaunchAgents/com.jeramo.pling.plist"
    if [ -f "$AGENT_PLIST" ]; then
        launchctl bootout "gui/$(id -u)" "$AGENT_PLIST" 2>/dev/null || true
        rm -f "$AGENT_PLIST"
    fi
    PLIST="/Library/LaunchDaemons/com.jeramo.pling.plist"
    sudo tee "$PLIST" > /dev/null <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>com.jeramo.pling</string>
    <key>ProgramArguments</key><array><string>${INSTALL_DIR}/pling</string><string>serve</string></array>
    <key>RunAtLoad</key><true/>
    <key>KeepAlive</key><true/>
    <key>StandardOutPath</key><string>/var/log/pling-launchd.log</string>
    <key>StandardErrorPath</key><string>/var/log/pling-launchd.log</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
    </dict>
</dict>
</plist>
EOF
    sudo chown root:wheel "$PLIST"
    sudo chmod 644 "$PLIST"
    sudo launchctl bootout system "$PLIST" 2>/dev/null || true
    sudo launchctl bootstrap system "$PLIST"
    echo "LaunchDaemon installed (starts at boot, before login)"
    echo "Note: --system has no GUI session, so Screen (video beta) and remote control are unavailable."

elif [ "$OS" = "darwin" ]; then
    # Remove any root LaunchDaemon from a prior --system install so the two
    # don't both run `pling serve` and fight over port 9876.
    SYS_PLIST="/Library/LaunchDaemons/com.jeramo.pling.plist"
    if [ -f "$SYS_PLIST" ]; then
        sudo launchctl bootout system "$SYS_PLIST" 2>/dev/null || true
        sudo rm -f "$SYS_PLIST"
    fi
    PLIST="$HOME/Library/LaunchAgents/com.jeramo.pling.plist"
    cat > "$PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>com.jeramo.pling</string>
    <key>ProgramArguments</key><array><string>${INSTALL_DIR}/pling</string><string>serve</string></array>
    <key>RunAtLoad</key><true/>
    <key>KeepAlive</key><true/>
    <key>StandardOutPath</key><string>${HOME}/Library/Logs/pling-launchd.log</string>
    <key>StandardErrorPath</key><string>${HOME}/Library/Logs/pling-launchd.log</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
    </dict>
</dict>
</plist>
EOF
    launchctl bootout "gui/$(id -u)" "$PLIST" 2>/dev/null || true
    launchctl bootstrap "gui/$(id -u)" "$PLIST"
    echo "LaunchAgent started"

    # The app's Screen feature captures natively on macOS 13+ (no ffmpeg);
    # only macOS 12 falls back to ffmpeg. It always needs Remote Login (the app
    # connects over SSH) plus Screen Recording + Accessibility for pling.
    if [ "$(sw_vers -productVersion 2>/dev/null | cut -d. -f1)" = "12" ] && ! command -v ffmpeg > /dev/null 2>&1 && [ ! -x /opt/homebrew/bin/ffmpeg ] && [ ! -x /usr/local/bin/ffmpeg ]; then
        echo "Note: on macOS 12 Screen needs ffmpeg. Install with: brew install ffmpeg"
    fi
    echo "For Screen: turn on Remote Login (System Settings > General > Sharing), then grant 'pling' Screen Recording + Accessibility in System Settings > Privacy & Security."
fi

echo "Done! pling is running."
echo "Try: pling status"

sleep 2
if command -v xdg-open > /dev/null 2>&1; then
    xdg-open "http://localhost:9876" 2>/dev/null &
elif command -v open > /dev/null 2>&1; then
    open "http://localhost:9876"
fi
