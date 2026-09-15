#!/usr/bin/env bash
set -euo pipefail

# Server IP: set DEPLOY_IP env or defaults to 192.168.0.234
: "${DEPLOY_IP:=192.168.0.234}"
: "${DEPLOY_USER:=aegis}"
: "${DEPLOY_PATH:=/opt/aegis}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
TARGET="$DEPLOY_USER@$DEPLOY_IP"
UNIT_NAME="aegis-server"

# Version stamp for client OTA (override with CLIENT_VERSION=...)
CLIENT_VERSION="${CLIENT_VERSION:-$(date -u +%Y%m%d%H%M%S)}"

# Non-interactive sudo (requires passwordless sudoers; see deploy/sudoers.aegis).
remote() {
    ssh -n -o BatchMode=yes "$TARGET" "$@"
}

remote_sudo() {
    remote "sudo -n $*"
}

require_sudo() {
    if ! remote_sudo true >/dev/null 2>&1; then
        echo "ERROR: passwordless sudo not available for $TARGET."
        echo "Install once (interactive):"
        echo "  scp $SCRIPT_DIR/sudoers.aegis $TARGET:/tmp/"
        echo "  ssh -t $TARGET 'sudo cp /tmp/sudoers.aegis /etc/sudoers.d/aegis-deploy && sudo chmod 440 /etc/sudoers.d/aegis-deploy'"
        exit 1
    fi
}

build_server() {
    cd "$PROJECT_ROOT"
    GOOS=linux GOARCH=amd64 go build -o aegis-server ./cmd/aegis-server
}

build_client_update() {
    cd "$PROJECT_ROOT"
    echo "Building Windows client version $CLIENT_VERSION"
    GOOS=windows GOARCH=amd64 go build \
        -ldflags "-X main.Version=${CLIENT_VERSION}" \
        -o aegis-client.exe ./cmd/aegis-client

    mkdir -p "$PROJECT_ROOT/deploy/updates-staging"
    cp "$PROJECT_ROOT/aegis-client.exe" "$PROJECT_ROOT/deploy/updates-staging/aegis-client.exe"

    if command -v shasum >/dev/null 2>&1; then
        SHA=$(shasum -a 256 "$PROJECT_ROOT/deploy/updates-staging/aegis-client.exe" | awk '{print $1}')
    else
        SHA=$(sha256sum "$PROJECT_ROOT/deploy/updates-staging/aegis-client.exe" | awk '{print $1}')
    fi

    cat > "$PROJECT_ROOT/deploy/updates-staging/client.json" <<EOF
{
  "version": "${CLIENT_VERSION}",
  "sha256": "${SHA}",
  "file": "aegis-client.exe"
}
EOF
    echo "Client update package ready: version=$CLIENT_VERSION sha256=$SHA"
}

publish_client_update() {
    remote "mkdir -p $DEPLOY_PATH/updates"
    scp "$PROJECT_ROOT/deploy/updates-staging/aegis-client.exe" "$TARGET:$DEPLOY_PATH/updates/"
    scp "$PROJECT_ROOT/deploy/updates-staging/client.json" "$TARGET:$DEPLOY_PATH/updates/"
    echo "Client update published to $TARGET:$DEPLOY_PATH/updates/"
}

# Stop unit + any orphan aegis-server (old nohup), so :8080 is free.
stop_server() {
    remote_sudo "systemctl stop $UNIT_NAME" || true
    # pgrep -x matches process name only — safe for this remote shell.
    remote 'pids=$(pgrep -x aegis-server || true); if [ -n "$pids" ]; then echo "killing orphans: $pids"; kill -9 $pids || true; fi'
    sleep 1
}

start_server() {
    remote_sudo "systemctl start $UNIT_NAME"
    sleep 1
    if ! remote "curl -sf -o /dev/null --connect-timeout 2 http://127.0.0.1:8080/"; then
        echo "server failed to answer on :8080"
        remote_sudo "systemctl status $UNIT_NAME --no-pager -l" || true
        remote "tail -n 30 $DEPLOY_PATH/server.log 2>/dev/null || true"
        exit 1
    fi
    remote_sudo "systemctl is-active $UNIT_NAME"
}

install_unit() {
    scp "$SCRIPT_DIR/aegis-server.service" "$TARGET:/tmp/aegis-server.service"
    remote_sudo "mv /tmp/aegis-server.service /etc/systemd/system/$UNIT_NAME.service"
    remote_sudo "systemctl daemon-reload"
    remote_sudo "systemctl enable $UNIT_NAME"
}

deploy_initial() {
    echo "=== Initial deploy to $TARGET:$DEPLOY_PATH ==="
    require_sudo
    build_server
    build_client_update

    remote_sudo "mkdir -p $DEPLOY_PATH && chown $DEPLOY_USER:$DEPLOY_USER $DEPLOY_PATH"
    scp "$PROJECT_ROOT/aegis-server" "$TARGET:$DEPLOY_PATH/"
    remote "chmod +x $DEPLOY_PATH/aegis-server && mkdir -p $DEPLOY_PATH/updates"
    install_unit
    start_server
    publish_client_update

    echo "Initial deploy complete. Service $UNIT_NAME is running."
    echo "Windows clients will pick up version $CLIENT_VERSION via OTA."
}

deploy_redeploy() {
    echo "=== Redeploy to $TARGET:$DEPLOY_PATH ==="
    require_sudo
    build_server
    build_client_update

    stop_server
    scp "$PROJECT_ROOT/aegis-server" "$TARGET:$DEPLOY_PATH/"
    remote "chmod +x $DEPLOY_PATH/aegis-server"
    # Keep unit file in sync (flags: --updates, timezone, …).
    install_unit
    start_server
    publish_client_update

    echo "Redeploy complete. Service $UNIT_NAME restarted."
    echo "Windows clients will pick up version $CLIENT_VERSION via OTA."
}

case "${1:-redeploy}" in
    initial)
        deploy_initial
        ;;
    redeploy)
        deploy_redeploy
        ;;
    client-only)
        echo "=== Publish client update only ==="
        build_client_update
        publish_client_update
        ;;
    *)
        echo "Usage: $0 {initial|redeploy|client-only}"
        echo "  initial     - first-time setup: create dir, copy binary, install systemd, start"
        echo "  redeploy    - systemctl stop → copy → start + publish Windows client OTA"
        echo "  client-only - only build/publish Windows client update"
        echo ""
        echo "Env: DEPLOY_IP, DEPLOY_USER, DEPLOY_PATH, CLIENT_VERSION"
        exit 1
        ;;
esac
