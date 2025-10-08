#!/bin/bash
# Entrypoint that sets up WireGuard interface based on hostname

set -e

HOSTNAME=$(hostname)
RELAY_ENDPOINT=${RELAY_ENDPOINT:-drio.sh:51820}

echo "Setting up WireGuard interface for $HOSTNAME..."
echo "Relay endpoint: $RELAY_ENDPOINT"

case "$HOSTNAME" in
    peer1)
        # Peer1: 192.168.4.1
        # Private key: 087ec6e14bbed210e7215cdc73468dfa23f080a1bfb8665b2fd809bd99d28379
        # Peer public key (peer2): c4c8e984c5322c8184c72265b92b250fdb63688705f504ba003c88f03393cf28

        ip link add dev wg0 type wireguard
        ip addr add 192.168.4.1/24 dev wg0

        # Write private key to temp file (wg requires no trailing newline)
        echo -n "087ec6e14bbed210e7215cdc73468dfa23f080a1bfb8665b2fd809bd99d28379" \
            | xxd -r -p | base64 -w0 > /tmp/wg0.key
  
        chmod 600 /tmp/wg0.key

        wg set wg0 private-key /tmp/wg0.key
        PEER_PUB=$(echo -n "c4c8e984c5322c8184c72265b92b250fdb63688705f504ba003c88f03393cf28" \
            | xxd -r -p | base64 -w0)
        wg set wg0 peer "$PEER_PUB"  \
            allowed-ips 192.168.4.2/24 \
            endpoint $RELAY_ENDPOINT \
            persistent-keepalive 5

        rm /tmp/wg0.key
        ip link set wg0 up
        echo "✓ WireGuard interface wg0 configured: 192.168.4.1"
        echo "  Peer: c4c8e984c5322c8184c72265b92b250fdb63688705f504ba003c88f03393cf28"
        echo "  Endpoint: $RELAY_ENDPOINT"
        ;;

    peer2)
        # Peer2: 192.168.4.2
        # Private key: 003ed5d73b55806c30de3f8a7bdab38af13539220533055e635690b8b87ad641
        # Peer public key (peer1): f928d4f6c1b86c12f2562c10b07c555c5c57fd00f59e90c8d8d88767271cbf7c

        ip link add dev wg0 type wireguard
        ip addr add 192.168.4.2/24 dev wg0

        # Write private key to temp file (wg requires no trailing newline)
        echo -n "003ed5d73b55806c30de3f8a7bdab38af13539220533055e635690b8b87ad641" \
            | xxd -r -p | base64 -w0 > /tmp/wg0.key
        chmod 600 /tmp/wg0.key

        wg set wg0 private-key /tmp/wg0.key
        PEER_PUB=$(echo -n "f928d4f6c1b86c12f2562c10b07c555c5c57fd00f59e90c8d8d88767271cbf7c" \
          | xxd -r -p | base64 -w0)
        wg set wg0 peer "$PEER_PUB"  \
            allowed-ips 192.168.4.1/32 \
            endpoint $RELAY_ENDPOINT \
            persistent-keepalive 5

        rm /tmp/wg0.key
        ip link set wg0 up
        echo "✓ WireGuard interface wg0 configured: 192.168.4.2"
        echo "  Peer: f928d4f6c1b86c12f2562c10b07c555c5c57fd00f59e90c8d8d88767271cbf7c"
        echo "  Endpoint: $RELAY_ENDPOINT"
        ;;

    *)
        echo "Unknown hostname: $HOSTNAME"
        echo "Expected peer1 or peer2"
        ;;
esac

echo ""
echo "Use 'wg show' to see interface status"
echo "Use 'ping 192.168.4.X' to test connectivity"
echo ""

# Execute whatever command was passed (usually /bin/bash)
exec "$@"
