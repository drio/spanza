#!/bin/bash
set -e

# DERP Production Setup Script
# This script helps set up a DERP server for production deployment

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DERPER_DIR="$(dirname "$SCRIPT_DIR")"
DERPER_BIN="$DERPER_DIR/derper"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

error() {
    echo -e "${RED}ERROR: $1${NC}" >&2
    exit 1
}

info() {
    echo -e "${GREEN}INFO: $1${NC}"
}

warning() {
    echo -e "${YELLOW}WARNING: $1${NC}"
}

usage() {
    cat << EOF
Usage: $0 <command> [options]

Commands:
    gen-key                 Generate persistent server key
    run <hostname>          Run server in production mode (foreground)
    install <hostname>      Install as systemd service
    uninstall              Remove systemd service
    test <hostname>        Test production server connectivity

Examples:
    $0 gen-key
    $0 run derp.example.com
    $0 install derp.example.com
    $0 test derp.example.com
    $0 uninstall

EOF
    exit 1
}

check_derper_binary() {
    if [ ! -f "$DERPER_BIN" ]; then
        error "derper binary not found at $DERPER_BIN. Run 'make build' first."
    fi
}

gen_key() {
    info "Generating persistent server key..."

    mkdir -p "$SCRIPT_DIR/config"

    if [ -f "$SCRIPT_DIR/config/derper.key" ]; then
        warning "Key already exists at $SCRIPT_DIR/config/derper.key"
        read -p "Delete and regenerate? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            info "Keeping existing key"
            return
        fi
        rm "$SCRIPT_DIR/config/derper.key"
    fi

    # Start derper briefly to generate key
    "$DERPER_BIN" -c "$SCRIPT_DIR/config/derper.key" --hostname=localhost &
    DERPER_PID=$!
    sleep 2
    kill $DERPER_PID 2>/dev/null || true
    wait $DERPER_PID 2>/dev/null || true

    if [ -f "$SCRIPT_DIR/config/derper.key" ]; then
        info "✓ Server key generated at $SCRIPT_DIR/config/derper.key"
        chmod 600 "$SCRIPT_DIR/config/derper.key"
    else
        error "Failed to generate server key"
    fi
}

run_prod() {
    local hostname="$1"

    if [ -z "$hostname" ]; then
        error "Hostname is required. Usage: $0 run <hostname>"
    fi

    check_derper_binary

    if [ ! -f "$SCRIPT_DIR/config/derper.key" ]; then
        error "No server key found. Run '$0 gen-key' first"
    fi

    mkdir -p "$SCRIPT_DIR/certs"

    info "Starting DERP server in production mode..."
    info "Hostname: $hostname"
    info "Port: 443 (requires root or CAP_NET_BIND_SERVICE)"
    info "Certificate mode: LetsEncrypt"
    info "Config: $SCRIPT_DIR/config/derper.key"
    info "Certs: $SCRIPT_DIR/certs"
    echo
    warning "This requires:"
    warning "  - Domain $hostname pointing to this server"
    warning "  - Ports 80 and 443 publicly accessible"
    warning "  - Root permissions or CAP_NET_BIND_SERVICE capability"
    echo

    if [ "$EUID" -ne 0 ]; then
        info "Running with sudo (required for port 443)..."
        sudo "$DERPER_BIN" \
            -c "$SCRIPT_DIR/config/derper.key" \
            --hostname="$hostname" \
            --certmode=letsencrypt \
            --certdir="$SCRIPT_DIR/certs" \
            -a :443
    else
        "$DERPER_BIN" \
            -c "$SCRIPT_DIR/config/derper.key" \
            --hostname="$hostname" \
            --certmode=letsencrypt \
            --certdir="$SCRIPT_DIR/certs" \
            -a :443
    fi
}

install_systemd() {
    local hostname="$1"

    if [ -z "$hostname" ]; then
        error "Hostname is required. Usage: $0 install <hostname>"
    fi

    if [ "$EUID" -ne 0 ]; then
        error "This command must be run as root. Use: sudo $0 install $hostname"
    fi

    check_derper_binary

    info "Installing DERP server as systemd service..."

    # Create user
    if ! id derper &>/dev/null; then
        useradd -r -s /bin/false derper
        info "Created derper user"
    fi

    # Create directories
    mkdir -p /var/lib/derper/certs
    chown derper:derper /var/lib/derper
    chown derper:derper /var/lib/derper/certs

    # Copy binary
    cp "$DERPER_BIN" /usr/local/bin/derper
    chown root:root /usr/local/bin/derper
    chmod 755 /usr/local/bin/derper
    info "Installed binary to /usr/local/bin/derper"

    # Copy or generate key
    if [ -f "$SCRIPT_DIR/config/derper.key" ]; then
        cp "$SCRIPT_DIR/config/derper.key" /var/lib/derper/derper.key
        chown derper:derper /var/lib/derper/derper.key
        chmod 600 /var/lib/derper/derper.key
        info "Copied server key to /var/lib/derper/derper.key"
    else
        warning "No key found at $SCRIPT_DIR/config/derper.key"
        info "Generating new key at /var/lib/derper/derper.key"
        sudo -u derper /usr/local/bin/derper -c /var/lib/derper/derper.key --hostname=localhost &
        DERPER_PID=$!
        sleep 2
        kill $DERPER_PID 2>/dev/null || true
        wait $DERPER_PID 2>/dev/null || true
        chmod 600 /var/lib/derper/derper.key
        chown derper:derper /var/lib/derper/derper.key
    fi

    # Create systemd service file
    cat > /etc/systemd/system/derper.service << EOF
[Unit]
Description=Tailscale DERP Server
After=network.target

[Service]
Type=simple
User=derper
ExecStart=/usr/local/bin/derper \\
  -c /var/lib/derper/derper.key \\
  --hostname=$hostname \\
  --certmode=letsencrypt \\
  --certdir=/var/lib/derper/certs \\
  -a :443
Restart=always
RestartSec=5
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    info "✓ Systemd service installed"
    echo
    info "To start the service:"
    echo "  sudo systemctl enable derper"
    echo "  sudo systemctl start derper"
    echo "  sudo systemctl status derper"
    echo
    info "To view logs:"
    echo "  sudo journalctl -u derper -f"
}

uninstall_systemd() {
    if [ "$EUID" -ne 0 ]; then
        error "This command must be run as root. Use: sudo $0 uninstall"
    fi

    info "Uninstalling DERP systemd service..."

    # Stop and disable service
    if systemctl is-active --quiet derper; then
        systemctl stop derper
        info "Stopped derper service"
    fi

    if systemctl is-enabled --quiet derper; then
        systemctl disable derper
        info "Disabled derper service"
    fi

    # Remove files
    rm -f /etc/systemd/system/derper.service
    rm -f /usr/local/bin/derper

    systemctl daemon-reload
    info "✓ Service removed"
    echo
    warning "Data directory /var/lib/derper still exists (contains keys/certs)"
    read -p "Delete /var/lib/derper? (y/N) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        rm -rf /var/lib/derper
        info "Deleted /var/lib/derper"
    fi

    read -p "Delete derper user? (y/N) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        userdel derper
        info "Deleted derper user"
    fi
}

test_server() {
    local hostname="$1"

    if [ -z "$hostname" ]; then
        error "Hostname is required. Usage: $0 test <hostname>"
    fi

    info "Testing DERP server at https://$hostname"
    echo

    # Test HTTPS root
    info "Testing HTTPS endpoint..."
    if curl -sf "https://$hostname" > /dev/null; then
        info "✓ HTTPS endpoint responding"
    else
        error "✗ HTTPS endpoint not responding"
    fi

    # Test DERP endpoint
    info "Testing DERP endpoint..."
    if curl -sf "https://$hostname/derp" > /dev/null; then
        info "✓ DERP endpoint responding"
    else
        error "✗ DERP endpoint not responding"
    fi

    # Check certificate
    info "Checking TLS certificate..."
    echo | openssl s_client -connect "$hostname:443" 2>/dev/null | \
        openssl x509 -noout -dates

    info "✓ Server is healthy!"
}

# Main command dispatch
case "${1:-}" in
    gen-key)
        check_derper_binary
        gen_key
        ;;
    run)
        run_prod "$2"
        ;;
    install)
        install_systemd "$2"
        ;;
    uninstall)
        uninstall_systemd
        ;;
    test)
        test_server "$2"
        ;;
    *)
        usage
        ;;
esac
