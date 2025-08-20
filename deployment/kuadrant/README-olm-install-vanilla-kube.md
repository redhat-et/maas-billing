# Installing Kuadrant with OLM on Vanilla Kubernetes

This guide shows how to install Kuadrant using Operator Lifecycle Manager (OLM) on vanilla Kubernetes clusters. This is required for accessing the latest Kuadrant operator images.

## Step 1: Install OLM (Operator Lifecycle Manager)

OLM is required to manage operators on vanilla Kubernetes. On OpenShift, OLM comes pre-installed.

```bash
# Download and install OLM v0.33.0
curl -L https://github.com/operator-framework/operator-lifecycle-manager/releases/download/v0.33.0/install.sh -o install.sh
chmod +x install.sh
./install.sh v0.33.0
```

Wait for OLM to be ready:

```bash
# Wait for OLM operators to be ready
kubectl wait --for=condition=Available deployment/olm-operator -n olm --timeout=300s
kubectl wait --for=condition=Available deployment/catalog-operator -n olm --timeout=300s
kubectl wait --for=condition=Available deployment/packageserver -n olm --timeout=300s
```

## Step 2: Install Kuadrant using OLM

Now that OLM is installed, you can use the OLM-based Kuadrant installation:

```bash
# Install Kuadrant operator via OLM
kubectl apply -k ./olm/install
```

Wait for Kuadrant operator to be ready (takes a couple of minutes for the deployment to begin):

```bash
# Wait for Kuadrant operator to be installed
kubectl wait --for=condition=Available deployment/kuadrant-operator-controller-manager -n kuadrant-system --timeout=300s
```

```shell
# Verify CatalogSource is ready (must be READY)
kubectl -n kuadrant-system get catalogsource kuadrant-operator-catalog -o jsonpath='{.status.connectionState.lastObservedState}{"\n"}'
kubectl -n kuadrant-system get pods -l olm.catalogSource=kuadrant-operator-catalog

# Check subscriptions and CSVs created by the install overlay
kubectl -n kuadrant-system get subscription
kubectl -n kuadrant-system get csv
kubectl -n gateway-system get subscription  
kubectl -n gateway-system get csv

# Configure Kuadrant instance
kubectl apply -k ./olm/configure

# Reinstall Istio to fix webhook certificate trust issues
./istio-install.sh uninstall
./istio-install.sh apply

# Completely remove Sail operator to prevent conflicting istiod deployments
kubectl -n gateway-system delete subscription sailoperator --ignore-not-found
kubectl -n gateway-system delete csv sailoperator.v0.1.0 --ignore-not-found
kubectl -n gateway-system delete deployment sail-operator --ignore-not-found
kubectl -n gateway-system delete deployment istiod --ignore-not-found

# Clean up stale certificate configmaps that prevent proper trust chain
kubectl -n llm delete configmap istio-ca-root-cert --ignore-not-found

# Reapply gateway configuration (was removed during Istio uninstall)
kubectl apply -f 02-gateway-configuration.yaml
kubectl apply -f 03-model-routing-domains.yaml

# Clean up any leftover replica sets to prevent pod spawning issues
kubectl -n llm delete replicaset -l gateway.networking.k8s.io/gateway-name=inference-gateway --ignore-not-found
kubectl -n llm rollout restart deployment/inference-gateway-istio

# Verify Kuadrant CR readiness
kubectl get kuadrant kuadrant -n kuadrant-system \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")].message}{"\n"}'

# Verify gateway pods connect to correct istiod (should be istio-system, not gateway-system)
kubectl -n llm logs deployment/inference-gateway-istio | grep "connected to"
```

## Step 3: Configure Kuadrant

Apply the Kuadrant configuration:

```bash
# Apply Kuadrant configuration
kubectl apply -f 04-kuadrant-operator.yaml
```
