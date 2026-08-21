#!/usr/bin/env bash
set -e

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

    # SHA256 (macOS shasum / Linux sha256sum)
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
    ssh -t "$TARGET" "sudo mkdir -p $DEPLOY_PATH/updates && sudo chown $DEPLOY_USER:$DEPLOY_USER $DEPLOY_PATH/updates"
    scp "$PROJECT_ROOT/deploy/updates-staging/aegis-client.exe" "$TARGET:$DEPLOY_PATH/updates/"
    scp "$PROJECT_ROOT/deploy/updates-staging/client.json" "$TARGET:$DEPLOY_PATH/updates/"
    echo "Client update published to $TARGET:$DEPLOY_PATH/updates/"
}

deploy_initial() {
    echo "=== Initial deploy to $TARGET:$DEPLOY_PATH ==="

    build_server
    build_client_update

    # Create directory, copy files, install systemd
    ssh -t "$TARGET" "sudo mkdir -p $DEPLOY_PATH && sudo chown $DEPLOY_USER:$DEPLOY_USER $DEPLOY_PATH"
    scp "$PROJECT_ROOT/aegis-server" "$TARGET:$DEPLOY_PATH/"
    scp "$SCRIPT_DIR/aegis-server.service" "$TARGET:/tmp/"
    ssh -t "$TARGET" "sudo mv /tmp/aegis-server.service /etc/systemd/system/ && sudo systemctl daemon-reload && sudo systemctl enable $UNIT_NAME && sudo systemctl start $UNIT_NAME"

    publish_client_update

    echo "Initial deploy complete. Service $UNIT_NAME is running."
    echo "Windows clients will pick up version $CLIENT_VERSION via OTA."
}

deploy_redeploy() {
    echo "=== Redeploy to $TARGET:$DEPLOY_PATH ==="

    build_server
    build_client_update

    # Stop service, copy binary, start
    ssh -t "$TARGET" "sudo systemctl stop $UNIT_NAME"
    scp "$PROJECT_ROOT/aegis-server" "$TARGET:$DEPLOY_PATH/"
    ssh -t "$TARGET" "sudo systemctl start $UNIT_NAME"

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
        echo "  redeploy    - update server + publish Windows client OTA package (default)"
        echo "  client-only - only build/publish Windows client update"
        echo ""
        echo "Env: DEPLOY_IP, DEPLOY_USER, DEPLOY_PATH, CLIENT_VERSION"
        exit 1
        ;;
esac
