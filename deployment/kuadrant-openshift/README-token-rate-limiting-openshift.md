# Models as a Service with Kuadrant

This repository demonstrates how to deploy a Models-as-a-Service platform using Kuadrant on Openshift. Kuadrant provides cloud-native API gateway capabilities using Istio and the Gateway API.

## Demos

These demos require a Kuadrant wasm-shim fork with metric support `oci://ghcr.io/nerdalert/wasm-shim:latest` until metrics are supported upstream discussed in [Question on mapping authorized_calls metrics to a user](https://github.com/Kuadrant/limitador/issues/434). Everything is upstream Kuadrant componentry accept for metrics collections and enabling a metrics scrape target in [nerdalert:wasm-shim:chargeback-wip](https://github.com/Kuadrant/wasm-shim/compare/main...nerdalert:wasm-shim:chargeback-wip).

### Demo:  Token Rate Limiting User Quotas

1. Generate requests across various users
2. Hit token caps on each of the user's policies based on their group
3. View token metrics in Prometheus 

<video src="https://github.com/user-attachments/assets/2c205809-b7fa-466f-b74a-e0c6c9c26569" controls></video>

### Demo: Token Rate Policy and Charge Metering

1. Increase `TokenRateLimitPolicy` quota
2. Generate continous user request load across users in the background
3. View token metrics and chargeback metering in Prometheus (Premium group: $0.008 per/token | Freemium group: $0.005 per/token)

<video src="https://github.com/user-attachments/assets/c08d3e39-3d70-49ff-a67e-1a062eb339b0" controls></video>

## Architecture Overview

**Gateway:** API Gateway + Istio/Envoy with Kuadrant policies integrated
**Models:** KServe InferenceServices (Included Qwen, Simulator)
**Authentication:** API Keys (simple) or Keycloak (Red Hat SSO)
**Rate Limiting:** Kuadrant RateLimitPolicy
**Observability:** Prometheus + Kuadrant Scrapes

### Key Components

- **Istio Service Mesh**: Provides the data plane for traffic management
- **Kuadrant Operator**: Manages API policies and traffic control
- **Limitador**: Rate limiting service with Redis backend
- **Authorino**: Authentication and authorization service
- **Gateway API**: Standard Kubernetes API for ingress traffic
- **KServe**: Model serving platform that creates model pods

## How Model Pods Get Created

**The flow that creates actual running model pods:**

```bash
1. Apply an InferenceService YAML
   ↓
2. KServe Controller sees the InferenceService
   ↓
3. KServe creates a Deployment for your model
   ↓
4. Deployment creates Pod(s) with:
   - GPU allocation
   - Model download from HuggingFace
   - vLLM or other serving runtime
   ↓
5. Pod starts serving model on port 8080
   ↓
6. Kube Service exposes the pod
   ↓
7. HTTPRoute creates domain-based routing (e.g., qwen3.maas.local, simulator.maas.local)
   ↓
8. Kuadrant policies protect each domain route
```

## Prerequisites

- Kubernetes cluster with admin access
- kubectl configured
- Running OpenShift cluster with Istio
- Kustomize

---

## Manual Deployment Instructions

```shell
git clone https://github.com/redhat-et/maas-billing.git
cd deployment/kuadrant-openshift
```

### 1. Install Istio and Gateway API

This presumes Istio is already installed on the OCP cluster.

Create additional namespaces:

```bash
kubectl apply -f 00-namespaces.yaml
```

### 2. Install KServe (for Model Serving)

**Note:** KServe requires cert-manager for webhook certificates.

```bash
# Install cert-manager first
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.18.2/cert-manager.yaml

# Wait for cert-manager to be ready
kubectl wait --for=condition=Available deployment/cert-manager -n cert-manager --timeout=300s
kubectl wait --for=condition=Available deployment/cert-manager-cainjector -n cert-manager --timeout=300s
kubectl wait --for=condition=Available deployment/cert-manager-webhook -n cert-manager --timeout=300s

# Install KServe CRDs and controller
kubectl apply --server-side -f https://github.com/kserve/kserve/releases/download/v0.15.2/kserve.yaml

# Wait for KServe controller to be ready
kubectl wait --for=condition=Available deployment/kserve-controller-manager -n kserve --timeout=300s

# Configure KServe for Gateway API integration
# For OpenShift clusters:
kubectl apply -f 01-kserve-config-openshift.yaml

# For local development:
# kubectl apply -f 01-kserve-config.yaml

# Restart KServe controller to pick up new configuration
kubectl rollout restart deployment/kserve-controller-manager -n kserve
kubectl wait --for=condition=Available deployment/kserve-controller-manager -n kserve --timeout=120s

# View inferenceervice configmap
kubectl get configmap inferenceservice-config -n kserve -o yaml

kubectl get configmap inferenceservice-config -n kserve \
  -o jsonpath='{.data.deploy}{"\n"}{.data.ingress}{"\n"}'

# Output
# {"defaultDeploymentMode": "RawDeployment"}
# {"enableGatewayApi": true, "kserveIngressGateway": "inference-gateway.llm"}
```

### 3. Configure Gateway and Routing

The configuration supports OpenShift cluster deployments.

#### For OpenShift Clusters:

Deploy the Gateway and routing configuration with OpenShift Routes:

```bash
kubectl apply -f 02-gateway-configuration.yaml
kubectl apply -f 03-model-routing-domains.yaml
kubectl apply -f 02a-openshift-routes.yaml
```

The manifests are pre-configured for the domain: `apps.summit-gpu.octo-emerging.redhataicoe.com`

**For different OpenShift clusters:** Update the hostnames in `02-gateway-configuration.yaml`, `03-model-routing-domains.yaml`, and `02a-openshift-routes.yaml` to match your cluster's ingress domain.

### 4. Install Kuadrant Operator

**Option 1: Using OLM with Latest Features (recommended for TokenRateLimitPolicy)**

Install the latest Kuadrant operator with TokenRateLimitPolicy support using OLM nightly builds:

```bash
# OpenShift already has OLM installed, so we can skip OLM installation

# Install Kuadrant operator with nightly catalog (includes latest features)
kubectl apply -k ./olm/install

# Verify CatalogSource is ready (must be READY)
kubectl -n kuadrant-system get catalogsource kuadrant-operator-catalog -o jsonpath='{.status.connectionState.lastObservedState}{"\n"}'
kubectl -n kuadrant-system get pods -l olm.catalogSource=kuadrant-operator-catalog

# Check subscriptions and CSVs created by the install overlay
kubectl -n kuadrant-system get subscription
kubectl -n kuadrant-system get csv
kubectl -n gateway-system get subscription 2>/dev/null || echo "No gateway-system subscriptions"
kubectl -n gateway-system get csv 2>/dev/null || echo "No gateway-system CSVs"

# Configure Kuadrant instance
kubectl apply -k ./olm/configure

# Watch operator-managed deployments settle
kubectl -n kuadrant-system get deploy
kubectl -n gateway-system get deploy 2>/dev/null || echo "No gateway-system deployments"

# !!! OpenShift-specific Sail operator cleanup !!!
# The OLM installation may include Sail operator which conflicts with existing OpenShift Service Mesh Istio
# Remove conflicting components without uninstalling existing Istio

# 1. Remove Sail operator components from gateway-system if they exist
kubectl -n gateway-system delete subscription sailoperator --ignore-not-found
kubectl -n gateway-system delete csv sailoperator.v0.1.0 --ignore-not-found
kubectl -n gateway-system delete deployment sail-operator --ignore-not-found
kubectl -n gateway-system delete deployment istiod --ignore-not-found

# 2. Clean up stale certificate configmaps that prevent proper trust chain
kubectl -n llm delete configmap istio-ca-root-cert --ignore-not-found

# 3. Ensure gateway configuration is applied (may need reapplication)
kubectl apply -f 02-gateway-configuration.yaml
kubectl apply -f 03-model-routing-domains.yaml
kubectl apply -f 02a-openshift-routes.yaml

# 4. Clean up any leftover replica sets to prevent pod spawning issues
kubectl -n llm delete replicaset -l gateway.networking.k8s.io/gateway-name=inference-gateway --ignore-not-found
kubectl -n llm rollout restart deployment/inference-gateway-istio 2>/dev/null || echo "Gateway deployment not yet created"

# Wait for Kuadrant operator to create the Kuadrant CR automatically
echo "Waiting for Kuadrant CR to be created by operator..."
for i in {1..30}; do
  if kubectl get kuadrant kuadrant -n kuadrant-system >/dev/null 2>&1; then
    echo "Kuadrant CR found, checking readiness..."
    break
  fi
  echo "Attempt $i/30: Kuadrant CR not found, waiting 10 seconds..."
  sleep 10
done

# Verify Kuadrant CR readiness
kubectl get kuadrant kuadrant -n kuadrant-system \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")].message}{"\n"}' 2>/dev/null || echo "Kuadrant CR not yet ready"

# If Kuadrant CR is still not ready, check the controller logs
kubectl -n kuadrant-system logs deployment/kuadrant-operator-controller-manager --tail=20

# Verify gateway pods connect to correct istiod (should be istio-system, not gateway-system)
kubectl -n llm logs deployment/inference-gateway-istio 2>/dev/null | grep "connected to" || echo "Gateway not yet running"
```

> **⚠️ OpenShift OLM Installation Note**: OpenShift comes with OLM pre-installed. The installation creates components that may conflict with existing Service Mesh Istio. The cleanup steps above ensure proper integration with OpenShift's existing Istio installation.

**Option 2: Using Helm (stable release, no TokenRateLimitPolicy support)**

```bash
helm repo add kuadrant https://kuadrant.io/helm-charts
helm repo update

helm install kuadrant-operator kuadrant/kuadrant-operator \
  --create-namespace \
  --namespace kuadrant-system

kubectl apply -f 04-kuadrant-operator.yaml

# Wait for the operator to be ready
kubectl wait --for=condition=Available deployment/kuadrant-operator-controller-manager -n kuadrant-system --timeout=300s

# If the status does not become ready try kicking the operator:
kubectl rollout restart deployment kuadrant-operator-controller-manager -n kuadrant-system
```

### 7. Deploy AI Models with KServe

#### Prerequisites

Deploy Security Context Constraints for OpenShift:

```bash
# Deploy ServiceAccount and SecurityContextConstraints
kubectl apply -f 02b-openshift-scc.yaml
```

#### Deploy Models

```bash
# Deploy the simulator model (lightweight, no GPU required)
kubectl apply -f ../model_serving/vllm-simulator-kserve-openshift.yaml

# Deploy the vLLM ServingRuntime for GPU models
kubectl apply -f ../model_serving/vllm-latest-runtime-openshift.yaml

# Deploy the Qwen3-0.6B model (requires GPU nodes)
kubectl apply -f ../model_serving/qwen3-0.6b-vllm-raw-openshift.yaml
```

#### Monitor Deployment

```bash
# Check InferenceService status
kubectl get inferenceservice -n llm

# Monitor pod deployment (GPU models take 5-10 minutes)
kubectl get pods -n llm -l serving.kserve.io/inferenceservice
```

**OpenDataHub Webhook Conflicts (ROSA clusters):**

```bash
# Remove conflicting OpenDataHub webhook if present
kubectl delete mutatingwebhookconfiguration mutating.odh-model-controller.opendatahub.io --ignore-not-found
```

### 8. Configure Authentication and Token-Based Rate Limiting

Deploy API key secrets, auth policies, and token-based rate limiting:

- Keycloak OIDC Authentication (Alternative to API Keys, see - [vanilla/dev deployment](../kuadrant))

```bash
# Create API key secrets
kubectl apply -f 05-api-key-secrets.yaml

# Apply API key-based auth policies
kubectl apply -f 06-auth-policies-apikey.yaml

# Apply TokenRateLimitPolicy for Gateway-level Token Rate Limiting
kubectl apply -f 08-token-rate-limit-policy.yaml

# Verify TokenRateLimitPolicy was created
kubectl get tokenratelimitpolicy gateway-token-rate-limits -n llm

# Verify WasmPlugin was generated automatically by Kuadrant
kubectl get wasmplugin -n llm

# Check if Kuadrant controller needs restart
kubectl rollout restart deployment kuadrant-operator-controller-manager -n kuadrant-system
```

### 9. Access Your Models

Your models are directly accessible via the OpenShift Routes (no port-forwarding needed):

- **Simulator**: `http://simulator-llm.apps.summit-gpu.octo-emerging.redhataicoe.com`
- **Qwen3**: `http://qwen3-llm.apps.summit-gpu.octo-emerging.redhataicoe.com`

### 10. Test Token-Based Rate Limiting

Test token-based rate limiting with manual curl commands that consume different amounts of tokens:

#### Test Free Tier (100 tokens per 1min)

```bash
# Small request consuming ~30 tokens
curl -s -H 'Authorization: APIKEY freeuser1_key' \
     -H 'Content-Type: application/json' \
     -d '{
       "model": "simulator-model",
       "messages": [{"role": "user", "content": "Hi"}],
       "max_tokens": 10
     }' \
     http://simulator-llm.apps.summit-gpu.octo-emerging.redhataicoe.com/v1/chat/completions

# Large request consuming ~530 tokens
curl -s -H 'Authorization: APIKEY freeuser1_key' \
     -H 'Content-Type: application/json' \
     -d '{
       "model": "simulator-model",
       "messages": [{"role": "user", "content": "Write a detailed explanation of quantum computing"}],
       "max_tokens": 500
     }' \
     http://simulator-llm.apps.summit-gpu.octo-emerging.redhataicoe.com/v1/chat/completions

# Test token exhaustion - this should return 429 after consuming 1000+ tokens
for i in {1..5}; do
  echo "Large request #$i:"
  curl -s -w "HTTP Status: %{http_code}\\n" \
    -H 'Authorization: APIKEY freeuser1_key' \
    -H 'Content-Type: application/json' \
    -d '{"model":"simulator-model","messages":[{"role":"user","content":"Generate a long detailed response about AI"}],"max_tokens":400}' \
    http://simulator-llm.apps.summit-gpu.octo-emerging.redhataicoe.com/v1/chat/completions
done
```

#### Test Premium Tier (500 tokens per 1min)

```bash
# Test Premium tier with Qwen3 model
curl -s -H 'Authorization: APIKEY premiumuser1_key' \
     -H 'Content-Type: application/json' \
     -d '{
       "model": "qwen3-0-6b-instruct",
       "messages": [{"role": "user", "content": "Explain why cats are better than dogs"}],
       "max_tokens": 200
     }' \
     http://qwen3-llm.apps.summit-gpu.octo-emerging.redhataicoe.com/v1/chat/completions
```

### 9. Deploy Observability and Token Usage Metrics

Deploy ServiceMonitors to integrate with OpenShift's existing Prometheus monitoring for token usage tracking:

```bash
# Deploy ServiceMonitors for Kuadrant components
kubectl apply -k kustomize/prometheus/

# Deploy Token Usage Metrics ServiceMonitor for Istio gateway
kubectl apply -f 09-token-rate-limit-servicemonitor-envoy-shim.yaml

# Enable user workload monitoring for llm namespace
kubectl label namespace llm openshift.io/user-monitoring=true

# Verify ServiceMonitors are created
kubectl get servicemonitor -n kuadrant-system
kubectl get servicemonitor -n llm
```

#### Query Token Usage Metrics from Prometheus

Access token usage metrics through OpenShift's user workload monitoring Prometheus:

```bash
# Query total token usage across all users
kubectl exec -n openshift-user-workload-monitoring prometheus-user-workload-0 -c prometheus -- \
  curl -s 'http://localhost:9090/api/v1/query' \
  --data-urlencode 'query=token_usage_total' | jq '.data.result'

# Query token usage by user and group
kubectl exec -n openshift-user-workload-monitoring prometheus-user-workload-0 -c prometheus -- \
  curl -s 'http://localhost:9090/api/v1/query' \
  --data-urlencode 'query=token_usage_with_user_and_group' | jq '.data.result'
```

Example response showing token usage by user:

```shell
# Query aggregated token usage by user and group
kubectl exec -n openshift-user-workload-monitoring prometheus-user-workload-0 -c prometheus -- \
  curl -s 'http://localhost:9090/api/v1/query' \
  --data-urlencode 'query=sum by (user, group) (token_usage_with_user_and_group)' | jq '.data.result'
```

Output:

```json
[
   {
      "metric": {
         "group": "free",
         "user": "freeuser1"
      },
      "value": [
         1754793040.817,
         "230"
      ]
   },
   {
      "metric": {
         "group": "premium",
         "user": "premiumuser1"
      },
      "value": [
         1754793040.817,
         "1037"
      ]
   },
   {
      "metric": {
         "group": "free",
         "user": "freeuser2"
      },
      "value": [
         1754793040.817,
         "240"
      ]
   },
   {
      "metric": {
         "group": "premium",
         "user": "premiumuser2"
      },
      "value": [
         1754793040.817,
         "1035"
      ]
   }
]
```

#### Raw Istio Metrics Access

For debugging, access raw Istio Envoy metrics directly:

```bash
# Get gateway pod name
GATEWAY_POD=$(kubectl -n llm get pods -l gateway.networking.k8s.io/gateway-name=inference-gateway -o jsonpath='{.items[0].metadata.name}')

# Scrape token usage metrics from Istio proxy
kubectl -n llm exec -it $GATEWAY_POD -c istio-proxy -- \
  curl -s http://127.0.0.1:15090/stats/prometheus | grep token_usage
```

### 10. Access Grafana Dashboard via OpenShift Route

Deploy the user workload monitoring data source and token usage dashboard:

```bash
# Deploy user workload monitoring data source for Grafana
kubectl apply -f kustomize/prometheus/user-workload-datasource.yaml

# Import token usage dashboard (requires Grafana UI or API)
# Dashboard JSON available at: kustomize/prometheus/token-dashboard-import.json

# Verify data source is created
kubectl get grafanadatasource -n llm-d-observability
```

Access Grafana dashboard:
- **Grafana URL**: https://grafana-route-llm-d-observability.apps.summit-gpu.octo-emerging.redhataicoe.com
- **Dashboard**: Import `kustomize/prometheus/token-dashboard-import.json` through Grafana UI

The dashboard provides:
- Total token usage across all users
- Token usage breakdown by user groups (free, premium)
- Top token consumers
- Token usage trends and rates
- Real-time monitoring with 30-second refresh

**Note:** This deployment uses OpenShift's built-in user workload monitoring instead of deploying a separate Prometheus instance. The ServiceMonitor automatically relabels the encoded metrics due to Envoy's wasm-proxy lack of support for labels in the ABI interface. This forces us to encode labels into the metric and then relabel them in the service monitor. 

More on the topic in this document ** → [redhat-et/kuadrant-llm-integration](https://github.com/redhat-et/kuadrant-llm-integration/blob/main/llm-d/wasm-plugin-metrics.md) **

## Troubleshooting

### View Logs

```bash
# Kuadrant operator logs
kubectl logs -n kuadrant-system deployment/kuadrant-operator-controller-manager

# Istio gateway logs
kubectl logs -n istio-system deployment/istio-ingressgateway

# Limitador logs
kubectl logs -n kuadrant-system deployment/limitador

# Authorino logs
kubectl logs -n kuadrant-system deployment/authorino
```

### Common Issues

- **502 Bad Gateway**: Check if model services are running and healthy
- **No Rate Limiting or Auth**: Kick the kuadrant-operator-controller-manager

## Customization

### Adjusting Rate Limits

Edit the RateLimitPolicy resources in `06-rate-limit-policies.yaml`:

```yaml
limits:
  "requests-per-minute":
    rates:
      - limit: 150  # Increase from 100
        duration: 1m
        unit: request
```

## Deploy WASM-Shim Image Replacement for User Based Rate and Token Limits Prometheus Metrics

When using TokenRateLimitPolicy with OLM-deployed Kuadrant, you may want to replace the default wasm-shim image with a custom version that provides enhanced logging for debugging token rate limiting behavior.

### Problem

The OLM-managed Kuadrant operator uses a specific wasm-shim image defined in its ClusterServiceVersion (CSV). Manual changes to WasmPlugin resources are automatically reverted by the operator's reconciliation process.

### Solution

To persistently use a custom wasm-shim image, you need to update the operator's environment variable at the CSV level, which OLM uses to generate WasmPlugin configurations.

### Step-by-Step Process

1. **Update the OLM ClusterServiceVersion to use custom image**:

   ```bash
   # Replace the default wasm-shim image in the CSV
   kubectl -n kuadrant-system patch csv kuadrant-operator.v0.0.0 --type='json' \
     -p='[{"op": "replace", "path": "/spec/install/spec/deployments/0/spec/template/spec/containers/0/env/1/value", "value": "oci://ghcr.io/nerdalert/wasm-shim:latest"}]'
   ```

2. **Restart the operator to pick up the new environment variable**:

   ```bash
   kubectl -n kuadrant-system rollout restart deployment/kuadrant-operator-controller-manager
   kubectl -n kuadrant-system wait --for=condition=Available deployment/kuadrant-operator-controller-manager --timeout=60s
   ```

3. **Verify the operator environment variable was updated**:

   ```bash
   kubectl -n kuadrant-system get deployment kuadrant-operator-controller-manager \
     -o jsonpath='{.spec.template.spec.containers[0].env[1]}' | jq
   ```

   Expected output:
   ```json
   {
     "name": "RELATED_IMAGE_WASMSHIM",
     "value": "oci://ghcr.io/nerdalert/wasm-shim:latest"
   }
   ```

4. **Force reconciliation of TokenRateLimitPolicy to regenerate WasmPlugin**:

   ```bash
   # Add annotation to trigger operator reconciliation
   kubectl -n llm patch tokenratelimitpolicy gateway-token-rate-limits --type='json' \
     -p='[{"op": "add", "path": "/metadata/annotations/force-reconcile", "value": "force-reconcile"}]'
   ```

5. **Verify WasmPlugin was updated with custom image**:

   ```bash
   kubectl -n llm get wasmplugin kuadrant-inference-gateway -o yaml | grep -A 3 -B 3 "url:"
   ```
   Expected output:
   ```yaml
   url: oci://ghcr.io/nerdalert/wasm-shim:latest
   ```

6. **Confirm gateway pods are fetching the custom image**:

   ```bash
   kubectl -n llm logs deployment/inference-gateway-istio --since=2m | grep "fetching image.*nerdalert"
   ```
   Expected output:
   ```
   info	wasm	fetching image nerdalert/wasm-shim from registry ghcr.io with tag latest
   ```

### Monitoring Custom WASM-Shim Logs

Once the custom wasm-shim is loaded, you can monitor its enhanced logging:

```bash
# Monitor all wasm-related logs from the gateway
kubectl -n llm logs deployment/inference-gateway-istio -f | grep wasm

# Monitor token rate limiting logs specifically
kubectl -n llm logs deployment/inference-gateway-istio -f | grep -E "(token|rate|limit)"

# Watch for enhanced logging from custom wasm-shim
kubectl -n llm logs deployment/inference-gateway-istio -f | grep -E "(nerdalert|custom|enhanced)"

# Monitor wasm plugin configuration and rule processing
kubectl -n llm logs deployment/inference-gateway-istio -f | grep -E "(plugin|config|rule)"

# Watch for rate limiting decisions and token consumption
kubectl -n llm logs deployment/inference-gateway-istio -f | grep -E "(OVER_LIMIT|allowed|denied|consumed)"
```

### ⚠️ Important Compatibility Note

**Custom wasm-shim images must support TokenRateLimitPolicy features**. If you encounter errors like:

```
wasm log: failed to parse plugin config: unknown variant `ratelimit-report`, expected one of `auth`, `ratelimit`, `ratelimit-check`
```

This indicates the custom image doesn't support token rate limiting. You must either:
- Use a newer version of the custom image that supports `ratelimit-report`
- Revert to the default image for token rate limiting functionality

### Restoring Default Image

If token rate limiting stops working with the custom image:

1. **Revert the CSV to use default image**:
   ```bash
   kubectl -n kuadrant-system patch csv kuadrant-operator.v0.0.0 --type='json' \
     -p='[{"op": "replace", "path": "/spec/install/spec/deployments/0/spec/template/spec/containers/0/env/1/value", "value": "oci://quay.io/kuadrant/wasm-shim:v7"}]'
   ```

2. **Restart operator and force reconciliation**:
   ```bash
   kubectl -n kuadrant-system rollout restart deployment/kuadrant-operator-controller-manager
   kubectl -n kuadrant-system wait --for=condition=Available deployment/kuadrant-operator-controller-manager --timeout=60s

   # Force WasmPlugin regeneration
   kubectl -n llm patch tokenratelimitpolicy gateway-token-rate-limits --type='json' \
     -p='[{"op": "replace", "path": "/metadata/annotations/force-reconcile", "value": "restore-default"}]'
   ```

3. **If WasmPlugin doesn't update, force deletion to regenerate**:
   ```bash
   kubectl -n llm delete wasmplugin kuadrant-inference-gateway
   # The operator will recreate it automatically
   ```

