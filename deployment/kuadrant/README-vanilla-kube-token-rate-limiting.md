# Installation for MaaS with Kuadrant via OLM on vanilla Kube

Installation for MaaS with Kuadrant via OLM on vanilla Kube. OLM latest is deployed via OLM from their CI deployments. For OpenShift token rate limiting policy instructions see [08-token-rate-limit-policy.yaml](..%2Fkuadrant-openshift%2F08-token-rate-limit-policy.yaml)


## Manual Deployment Instructions

This is useful for Kind or Minikube deployments.

Add the following to `/etc/hosts`

```shell
# Models-as-a-Service local domains
127.0.0.1    qwen3.maas.local
127.0.0.1    simulator.maas.localz
```

### 1. Install Istio and Gateway API

Install Istio and Gateway API CRDs using the provided script:

- Install Gateway API CRDs
- Install Istio base components and Istiod

```bash
chmod +x istio-install.sh
./istio-install.sh apply
```

This manifest will create the required namespaces (`llm` and `llm-observability`)

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
kubectl apply -f 01-kserve-config.yaml

# Restart KServe controller to pick up new configuration
kubectl rollout restart deployment/kserve-controller-manager -n kserve
kubectl wait --for=condition=Available deployment/kserve-controller-manager -n kserve --timeout=120s

# View inferenceervice configmap
kubectl get configmap inferenceservice-config -n kserve -o yaml

kubectl get configmap inferenceservice-config -n kserve \
  -o jsonpath='{.data.deploy}{"\n"}{.data.ingress}{"\n"}'
```

### 3. Configure Gateway and Routing

The configuration is pre-configured for domain-based routing. Deploy the Gateway and routing configuration:

```bash
kubectl apply -f 02-gateway-configuration.yaml
kubectl apply -f 03-model-routing-domains.yaml
```

**Note:** If you want to use a different domain, update the hostnames in the files before applying.

### 4. Install Kuadrant Operator

**Option 1: Using OLM with Latest Features (recommended)**

Install the latest Kuadrant operator with TokenRateLimitPolicy support using OLM nightly builds:

➡️ **Follow these instructions for OLM installation on vanilla kube [README-olm-install-vanilla-kube.md](README-olm-install-vanilla-kube.md)**

### 5. Deploy AI Models with KServe

> Option 1 Deploy models using KServe InferenceServices on a GPU accelerator:
> There is an added example of how to set the runtime with kserve via `vllm-latest-runtime.yaml`

```bash
# Deploy the latest vLLM ServingRuntime with Qwen3 support
kubectl apply -f ../model_serving/vllm-latest-runtime.yaml

# Deploy the Qwen3-0.6B model (recommended for testing)
kubectl apply -f ../model_serving/qwen3-0.6b-vllm-raw.yaml

# Monitor InferenceService deployment status
kubectl get inferenceservice -n llm

# Watch model deployment (takes 5-10 minutes for model download)
kubectl describe inferenceservice qwen3-0-6b-instruct -n llm

# Check if pods are running (may take 5-15 minutes for model downloads)
kubectl get pods -n llm -l serving.kserve.io/inferenceservice

# Follow logs to see model loading progress
kubectl logs -n llm -l serving.kserve.io/inferenceservice -c kserve-container -f

# Wait for model to be ready
kubectl wait --for=condition=Ready inferenceservice qwen3-0-6b-instruct -n llm --timeout=900s
```

> Option 2 - If in a KIND environment or non-GPU use:

```shell
kubectl apply -f ../model_serving/vllm-simulator-kserve.yaml
```

Check that the inference service comes up:

```shell
kubectl get inferenceservice
NAME             URL                                    READY   PREV   LATEST   PREVROLLEDOUTREVISION   LATESTREADYREVISION   AGE
vllm-simulator   http://vllm-simulator-llm.maas.local   True                                                                  19s
```

### 6. Configure Authentication and Rate Limiting

Deploy API key secrets, auth policies, and rate limiting:

```bash
# Create API key secrets
kubectl apply -f 05-api-key-secrets.yaml

# Apply API key-based auth policies
kubectl apply -f 06-auth-policies-apikey.yaml

# Kick the kuadrant controller if you dont see a limitador-limitador deployment or no rate-limiting
kubectl rollout restart deployment kuadrant-operator-controller-manager -n kuadrant-system
```

**Follow the instructions here to deploy the patched Kuadrant operator and wasm shim for token user metric support: [Instructions for enabling and disabling metrics in a running Kuadrant OLM deployment](https://gist.github.com/nerdalert/0df4874cdc74c8f676686ce77f352f7b)**

Then apply the token rate policy. Note: the token quota is high since for benchmarking, lower the `limit:` policy to validate quota violations.

```shell
# Apply TokenRateLimitPolicy for Gateway-level Token Rate Limiting
kubectl apply -f 08-token-rate-limit-policy.yaml
kubectl rollout restart deployment kuadrant-operator-controller-manager -n kuadrant-system
```

### 7. Start Port Forwarding for Local Access

If running on kind/minikube, you need port forwarding to access the models:

```bash
# Port-forward to Kuadrant gateway (REQUIRED for authentication)
kubectl port-forward -n llm svc/inference-gateway-istio 8000:80 &
```

### 8. Test the MaaS API

Test all user tiers and rate limits with the automated script:

```bash
# Test simulator model (default)
./scripts/test-request-limits.sh

# Test qwen3 model when ready
./scripts/test-request-limits.sh --host qwen3.maas.local --model qwen3-0-6b-instruct
```

### Query Metrics via the Istio Prom API

> 🚨 See this issue for status on getting per user scrape data for authorization and limits with Kudrant [Question on mapping authorized_calls metrics to a user](https://github.com/Kuadrant/limitador/issues/434)

This will demonstrate the Kuadrant limits being exposed through the Istio metrics.

```bash
kubectl -n llm exec deployment/inference-gateway-istio -c istio-proxy -- curl -s http://127.0.0.1:15090/stats/prometheus | grep token
```

Example output showing rate limiting in action:

```bash
Host    : simulator.maas.local
Model ID: simulator-model

=== Free User (5 requests per 2min) ===
Free req #1  -> 200
Free req #2  -> 200
Free req #3  -> 200
Free req #4  -> 200
Free req #5  -> 200
Free req #6  -> 429
Free req #7  -> 429

=== Premium User 1 (20 requests per 2min) ===
Premium1 req #1  -> 200
Premium1 req #2  -> 200
...
Premium1 req #20 -> 200
Premium1 req #21 -> 429
Premium1 req #22 -> 429

=== Premium User 2 (20 requests per 2min) ===
Premium2 req #1  -> 200
...
Premium2 req #20 -> 200
Premium2 req #21 -> 429
Premium2 req #22 -> 429

=== Second Free User (5 requests per 2min) ===
Free2 req #1  -> 200
...
Free2 req #5  -> 200
Free2 req #6  -> 429
Free2 req #7  -> 429
```

Test individual models with manual curl commands:

**Simulator Model:**

```bash
# Single request test
curl -s -H 'Authorization: APIKEY freeuser1_key' \
     -H 'Content-Type: application/json' \
     -d '{"model":"simulator-model","messages":[{"role":"user","content":"Hello!"}]}' \
     http://simulator.maas.local:8000/v1/chat/completions

# Test rate limiting (Free tier: 5 requests per 2min)
for i in {1..7}; do
  printf "Free tier request #%-2s -> " "$i"
  curl -s -o /dev/null -w "%{http_code}\n" \
       -X POST http://simulator.maas.local:8000/v1/chat/completions \
       -H 'Authorization: APIKEY freeuser1_key' \
       -H 'Content-Type: application/json' \
       -d '{"model":"simulator-model","messages":[{"role":"user","content":"Test request"}],"max_tokens":10}'
done
```

**Qwen3 Model:**

```bash
# Single request test
curl -s -H 'Authorization: APIKEY premiumuser1_key' \
     -H 'Content-Type: application/json' \
     -d '{"model":"qwen3-0-6b-instruct","messages":[{"role":"user","content":"Hello!"}]}' \
     http://qwen3.maas.local:8000/v1/chat/completions

# Test rate limiting (Premium tier: 20 requests per 2min)
for i in {1..22}; do
  printf "Premium tier request #%-2s -> " "$i"
  curl -s -o /dev/null -w "%{http_code}\n" \
       -X POST http://qwen3.maas.local:8000/v1/chat/completions \
       -H 'Authorization: APIKEY premiumuser1_key' \
       -H 'Content-Type: application/json' \
       -d '{"model":"qwen3-0-6b-instruct","messages":[{"role":"user","content":"Test request"}],"max_tokens":10}'
done
```

### 9. Deploy Observability

Deploy Prometheus and monitoring components:

```bash
# Install Prometheus Operator
kubectl apply --server-side --field-manager=quickstart-installer -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/master/bundle.yaml

# Wait for Prometheus Operator to be ready
kubectl wait --for=condition=Available deployment/prometheus-operator -n default --timeout=300s

# From models-aas/deployment/kuadrant Kuadrant prometheus observability
kubectl apply -k kustomize/prometheus/

# Wait for Prometheus to be ready
kubectl wait --for=condition=Running prometheus/models-aas-observability -n llm-observability --timeout=300s

# Port-forward to access Prometheus UI
kubectl port-forward -n llm-observability svc/models-aas-observability 9090:9090 &

# Access Prometheus at http://localhost:9090
```
