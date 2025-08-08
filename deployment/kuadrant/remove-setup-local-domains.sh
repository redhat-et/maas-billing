#!/bin/bash
# Setup script for local domain-based testing
# Creates /etc/hosts entries for model subdomains

set -e

DOMAINS=(
    "qwen3.maas.local"
    "simulator.maas.local"
)

setup_hosts() {
    echo "🔧 Setting up local domain names in /etc/hosts..."
    
    # Backup current hosts file
    sudo cp /etc/hosts /etc/hosts.backup.$(date +%Y%m%d_%H%M%S)
    
    # Remove existing maas.local entries
    sudo sed -i '/\.maas\.local/d' /etc/hosts
    
    # Add new entries
    echo "" | sudo tee -a /etc/hosts
    echo "# Models-as-a-Service local domains" | sudo tee -a /etc/hosts
    for domain in "${DOMAINS[@]}"; do
        echo "127.0.0.1    $domain" | sudo tee -a /etc/hosts
    done
    echo "" | sudo tee -a /etc/hosts
    
    echo "✅ Local domains configured:"
    for domain in "${DOMAINS[@]}"; do
        echo "   - http://$domain:8000"
    done
}

cleanup_hosts() {
    echo "🧹 Removing maas.local entries from /etc/hosts..."
    sudo sed -i '/\.maas\.local/d' /etc/hosts
    sudo sed -i '/# Models-as-a-Service local domains/d' /etc/hosts
    echo "✅ Cleanup complete"
}

case "$1" in
    setup)
        setup_hosts
        ;;
    cleanup)
        cleanup_hosts
        ;;
    *)
        echo "Usage: $0 {setup|cleanup}"
        echo ""
        echo "setup   - Add model domains to /etc/hosts"
        echo "cleanup - Remove model domains from /etc/hosts"
        echo ""
        echo "Available domains after setup:"
        for domain in "${DOMAINS[@]}"; do
            echo "  - http://$domain:8000"
        done
        exit 1
        ;;
esac
