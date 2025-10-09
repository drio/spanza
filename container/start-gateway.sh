#!/bin/bash
# Helper script to start Spanza gateway

set -e

HOSTNAME=$(hostname)
DERP_URL=${DERP_URL:-https://derp1.tailscale.com}

case "$HOSTNAME" in
    peer1)
        KEY_FILE="/workspace/peer1-derp.key"
        REMOTE_PUBKEY="nodekey:e3603e7b1d8024bad24da4c413b5989211c4f8e5ead29660f05addaa454e810b"
        ;;
    peer2)
        KEY_FILE="/workspace/peer2-derp.key"
        REMOTE_PUBKEY="nodekey:4b115ea75d1aeb08d489d9b9015f4b8228a60e1cfe4e231332e29bc4da71f659"
        ;;
    *)
        echo "Unknown hostname: $HOSTNAME"
        exit 1
        ;;
esac

echo "=========================================="
echo "Spanza Gateway Starter"
echo "=========================================="
echo ""

echo "Starting Spanza gateway..."
echo "  DERP server: $DERP_URL"
echo "  Our key: $OUR_PUBKEY"
echo "  Remote peer: $REMOTE_PUBKEY"
echo ""

exec /workspace/spanza \
    --key-file "$KEY_FILE" \
    --derp-url "$DERP_URL" \
    --remote-peer "$REMOTE_PUBKEY" \
    --listen :51821 \
    --wg-endpoint 127.0.0.1:51820 \
    --verbose
