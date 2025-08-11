#!/bin/bash

# Istio Installation Script
# This script installs Istio using Helm with a specific version for load balancing support
# What this installs:
# - Gateway API CRDs (for HTTPRoute support)
# - Istio base components
# - Istiod control plane
# - Creates the specified namespaces

set -e

MODE=${1:-apply}
TAG=1.26.2
HUB=gcr.io/istio-release
OCP=false

# Default namespace values, can be overridden by environment variables
VLLM_NAMESPACE=${VLLM_NAMESPACE:-llm}
OBSERVABILITY_NAMESPACE=${OBSERVABILITY_NAMESPACE:-llm-observability}
OCP=${OCP:-false}

echo "Istio installation mode: $MODE"
echo "Using tag: $TAG"
echo "Using hub: $HUB"
echo "vLLM namespace: $VLLM_NAMESPACE"
echo "Observability namespace: $OBSERVABILITY_NAMESPACE"
if [[ "$OCP" == true ]]; then
    echo "Skipping Istio installation on OCP..."
    return
fi

if [[ "$MODE" == "apply" ]]; then
    echo "Installing Istio..."

    # Install Gateway API CRDs first
    echo "Installing Gateway API CRDs..."
    kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.3.0/standard-install.yaml

    # Install Istio base
    echo "Installing Istio base..."
    helm upgrade -i istio-base oci://$HUB/charts/base --version $TAG -n istio-system --create-namespace

    # Install Istiod
    echo "Installing Istiod..."
    helm upgrade -i istiod oci://$HUB/charts/istiod --version $TAG -n istio-system --set tag=$TAG --set hub=$HUB --wait

    # Create namespaces if they don't exist
    echo "Creating $VLLM_NAMESPACE namespace..."
    kubectl create namespace "$VLLM_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
    echo "Creating $OBSERVABILITY_NAMESPACE namespace..."
    kubectl create namespace "$OBSERVABILITY_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

    echo "Istio installation completed successfully!"
    echo "Gateway API CRDs and Istio are now installed."
    echo "You can now deploy your applications with Istio Gateway and HTTPRoute support."

elif [[ "$MODE" == "uninstall" ]]; then
    echo "Uninstalling Istio..."

    # Uninstall Helm releases
    helm uninstall istiod --ignore-not-found --namespace istio-system || true
    helm uninstall istio-base --ignore-not-found --namespace istio-system || true

    # Clean up any remaining resources
    helm template istio-base oci://$HUB/charts/base --version $TAG -n istio-system | kubectl delete -f - --ignore-not-found || true

    # Clean up Gateway API CRDs (optional - comment out if you want to keep them)
    echo "Cleaning up Gateway API CRDs..."
    kubectl delete -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.3.0/standard-install.yaml --ignore-not-found || true

    echo "Istio uninstallation completed!"

else
    echo "Usage: $0 [apply|uninstall]"
    echo "  apply     - Install Istio with Gateway API CRDs (default)"
    echo "  uninstall - Remove Istio and Gateway API CRDs"
    exit 1
fi