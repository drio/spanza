#!/bin/bash
# Script to enable/disable restrictive firewall for testing DERP relay
#
# This simulates a restrictive corporate firewall that blocks all UDP traffic,
# forcing WireGuard to relay through DERP over HTTPS (port 443 only).

set -e

case "${1:-}" in
    enable)
        echo "Enabling restrictive firewall (blocking all UDP)..."
        ufw --force enable
        ufw default deny outgoing
        ufw default deny incoming
        ufw allow out 443/tcp comment 'Allow HTTPS for DERP'
        ufw allow in 443/tcp comment 'Allow HTTPS responses'
        ufw allow out 53 comment 'Allow DNS'
        ufw allow in 53 comment 'Allow DNS responses'
        ufw allow in on wg0 comment 'Allow WireGuard interface'
        ufw allow out on wg0 comment 'Allow WireGuard interface'
        echo ""
        echo "✓ Firewall enabled - all UDP blocked, only HTTPS (443) allowed"
        echo "  WireGuard must now relay through DERP over HTTPS"
        echo ""
        ufw status verbose
        ;;

    disable)
        echo "Disabling firewall..."
        ufw --force reset
        echo "✓ Firewall disabled - all traffic allowed"
        ;;

    status)
        ufw status verbose
        ;;

    *)
        echo "Usage: $0 {enable|disable|status}"
        echo ""
        echo "Commands:"
        echo "  enable  - Block all UDP, force DERP relay over HTTPS"
        echo "  disable - Remove all firewall rules"
        echo "  status  - Show current firewall status"
        exit 1
        ;;
esac
