#!/bin/bash
# Entrypoint that sets up WireGuard interface and Spanza gateway
#
# To test DERP relay when all UDP traffic is blocked (simulates restrictive firewall):
#   ./firewall-test.sh enable   - Block all UDP, force DERP over HTTPS
#   ./firewall-test.sh disable  - Remove firewall rules
#   ./firewall-test.sh status   - Show firewall status

set -e

HOSTNAME=$(hostname)
DERP_URL=${DERP_URL:-https://derp1.tailscale.com}

echo "Setting up WireGuard + Spanza gateway for $HOSTNAME..."
echo "DERP server: $DERP_URL"

case "$HOSTNAME" in
    peer1)
        # Peer1: 192.168.4.1
        # WG Private key: 087ec6e14bbed210e7215cdc73468dfa23f080a1bfb8665b2fd809bd99d28379
        # WG Peer public key (peer2): c4c8e984c5322c8184c72265b92b250fdb63688705f504ba003c88f03393cf28

        ip link add dev wg0 type wireguard
        ip addr add 192.168.4.1/24 dev wg0

        echo -n "087ec6e14bbed210e7215cdc73468dfa23f080a1bfb8665b2fd809bd99d28379" \
            | xxd -r -p | base64 -w0 > /tmp/wg0.key
        chmod 600 /tmp/wg0.key

        wg set wg0 private-key /tmp/wg0.key listen-port 51820
        PEER_PUB=$(echo -n "c4c8e984c5322c8184c72265b92b250fdb63688705f504ba003c88f03393cf28" \
            | xxd -r -p | base64 -w0)

        # Point WireGuard to local Spanza gateway (not remote peer directly)
        wg set wg0 peer "$PEER_PUB"  \
            allowed-ips 192.168.4.2/32 \
            endpoint 127.0.0.1:51821 \
            persistent-keepalive 25

        rm /tmp/wg0.key
        ip link set wg0 up

        echo "✓ WireGuard wg0: 192.168.4.1 → local gateway at 127.0.0.1:51821"
        echo ""
        echo "📋 DERP Public Key: nodekey:4b115ea75d1aeb08d489d9b9015f4b8228a60e1cfe4e231332e29bc4da71f659"

        PING_TARGET="192.168.4.2"
        ;;

    peer2)
        # Peer2: 192.168.4.2
        # WG Private key: 003ed5d73b55806c30de3f8a7bdab38af13539220533055e635690b8b87ad641
        # WG Peer public key (peer1): f928d4f6c1b86c12f2562c10b07c555c5c57fd00f59e90c8d8d88767271cbf7c

        ip link add dev wg0 type wireguard
        ip addr add 192.168.4.2/24 dev wg0

        echo -n "003ed5d73b55806c30de3f8a7bdab38af13539220533055e635690b8b87ad641" \
            | xxd -r -p | base64 -w0 > /tmp/wg0.key
        chmod 600 /tmp/wg0.key

        wg set wg0 private-key /tmp/wg0.key listen-port 51820
        PEER_PUB=$(echo -n "f928d4f6c1b86c12f2562c10b07c555c5c57fd00f59e90c8d8d88767271cbf7c" \
          | xxd -r -p | base64 -w0)

        # Point WireGuard to local Spanza gateway (not remote peer directly)
        wg set wg0 peer "$PEER_PUB"  \
            allowed-ips 192.168.4.1/32 \
            endpoint 127.0.0.1:51821 \
            persistent-keepalive 25

        rm /tmp/wg0.key
        ip link set wg0 up

        echo "✓ WireGuard wg0: 192.168.4.2 → local gateway at 127.0.0.1:51821"
        echo ""
        echo "📋 DERP Public Key: nodekey:e3603e7b1d8024bad24da4c413b5989211c4f8e5ead29660f05addaa454e810b"

        PING_TARGET="192.168.4.1"
        ;;

    *)
        echo "Unknown hostname: $HOSTNAME"
        echo "Expected peer1 or peer2"
        exit 1
        ;;
esac

echo ""
echo "=========================================="
echo "Setup complete!"
echo "=========================================="
echo ""
echo "Start gateway:"
echo "  ./start-gateway.sh > /tmp/gateway.log 2>&1 &"
echo ""
echo "Test connectivity:"
echo "  ping $PING_TARGET"
echo ""
echo "View gateway logs:"
echo "  tail -f /tmp/gateway.log"
echo ""
echo "Test firewall (blocks all UDP):"
echo "  ./firewall-test.sh enable"
echo "=========================================="
echo ""

exec /bin/bash
