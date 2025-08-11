#!/usr/bin/env bash
set -euo pipefail

################################################################################
# Models-as-a-Service (MaaS) + Kuadrant one-shot installer
#
# Flags
#   --simulator           Deploy only the vLLM simulator (CPU/KIND clusters)
#   --qwen3               Deploy only the Qwen3-0.6 B model (GPU cluster)
#   --install-all-models  Deploy both simulator and Qwen3
#   --deploy-kind          Spin up a kind cluster named llm-maas and deploy the
#                         simulator model into it
#
# The script must be run from  deployment/kuadrant  (it relies on relative paths)
################################################################################

NAMESPACE="llm"
MODEL_TYPE=""
DEPLOY_KIND=false
SKIP_METRICS=false
OCP=false

usage() {
  cat <<EOF
Usage: $0 [--simulator|--qwen3|--install-all-models|--deploy-kind] [--skip-metrics]

Options
  --simulator            Deploy vLLM simulator (no GPU required)
  --qwen3                Deploy Qwen3-0.6B model (GPU required)
  --install-all-models   Deploy both simulator and Qwen3
  --deploy-kind           Create a kind cluster named llm-maas and deploy the simulator model
  --skip-metrics         Skip Prometheus observability deployment
  --ocp                  Deploy to OpenShift cluster
Examples
  $0 --simulator
  $0 --qwen3 --skip-metrics
  $0 --install-all-models
  $0 --deploy-kind
  $0 --simulator --ocp
EOF
  exit 1
}

# ────────────────────────────── flag parsing ──────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --simulator)           MODEL_TYPE="simulator" ; shift ;;
    --qwen3)               MODEL_TYPE="qwen3"     ; shift ;;
    --install-all-models)  MODEL_TYPE="all"       ; shift ;;
    --deploy-kind)         DEPLOY_KIND=true; MODEL_TYPE="simulator" ; shift ;;
    --skip-metrics)        SKIP_METRICS=true ; shift ;;
    -h|--help)             usage ;;
    --ocp)                 OCP=true ; shift ;;
    *) echo "❌ Unknown option: $1"; usage ;;
  esac
done

[[ -z "$MODEL_TYPE" ]] && { echo "❌ Must specify a model flag"; usage; }

# ────────────────────────────── sanity checks ─────────────────────────────────
if [[ ! -f "02-gateway-configuration.yaml" ]]; then
  echo "❌ Run this script from deployment/kuadrant"
  exit 1
fi

# ────────────────────────────── optional kind cluster ─────────────────────────
if [[ "$DEPLOY_KIND" == true ]]; then
  echo "🔧 Creating kind cluster 'llm-maas' (if absent)"
  if ! kind get clusters | grep -q '^llm-maas$'; then
    kind create cluster --name llm-maas
  else
    echo "ℹ️  kind cluster 'llm-maas' already exists; reusing"
  fi
fi

echo
echo "🚀 MaaS installation started"
echo "📦  Model selection: $MODEL_TYPE"
echo

# ────────────────────────────── 1. Istio / Gateway API ────────────────────────
echo "🔧 1. Installing Istio & Gateway API"
chmod +x istio-install.sh
if [[ "$OCP" == true ]]; then
  # I keep running into issues because it is installed by servicemesh already
  echo "Skipping Istio installation on OCP..."
else
  ./istio-install.sh apply
fi

# ────────────────────────────── 2. Namespaces ────────────────────────────────
echo "🔧 2. Creating namespaces"
kubectl apply -f 00-namespaces.yaml

# ────────────────────────────── 3. cert-manager & KServe ─────────────────────
echo "🔧 3. Installing cert-manager"
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.18.2/cert-manager.yaml

echo "⏳   Waiting for cert-manager to be ready"
kubectl wait --for=condition=Available deployment/cert-manager            -n cert-manager --timeout=300s
kubectl wait --for=condition=Available deployment/cert-manager-cainjector -n cert-manager --timeout=300s
kubectl wait --for=condition=Available deployment/cert-manager-webhook   -n cert-manager --timeout=300s

if [[ "$OCP" == true ]]; then
  echo "Validating RHOAI operator is installed and DataScienceCluster is ready"
  kubectl wait --for=condition=Ready DataScienceCluster -l 'app.kubernetes.io/created-by=rhods-operator' -n redhat-ods-operator --timeout=300s
  echo "🔧 Skipping KServe setup, Openshift Serverless operator handles this (Installed by the RHOAI operator)"

  echo "🔧 Updating ServiceMeshControlPlane for RHOAI to enable Gateway API"
  kubectl patch smcp/data-science-smcp -n istio-system --type=merge -p '{"spec":{"mode":"ClusterWide"}}'
else
  echo "🔧 Installing KServe"
  kubectl apply --server-side -f https://github.com/kserve/kserve/releases/download/v0.15.2/kserve.yaml

  echo "⏳   Waiting for KServe controller"
  kubectl wait --for=condition=Available deployment/kserve-controller-manager -n kserve --timeout=300s
fi

echo "🔧 Configuring KServe for Gateway API"
if [[ "$OCP" == true ]]; then
  # Removing this for now, not sure if we need to make this these updates in order to get the gateway api to work later
  # kubectl apply -f 01-kserve-config.yaml
  echo "🔧 Skipping KServe config, Openshift Serverless operator handles this (Installed by the RHOAI operator)"
  echo "🔧 Applying OpenShift Security Context Constraints"
  kubectl apply -f 02b-openshift-scc.yaml
  echo "🔧 Creating Routes"
  kubectl apply -f 02a-openshift-routes.yaml
else
  kubectl apply -f 01-kserve-config.yaml
  kubectl rollout restart deployment/kserve-controller-manager -n kserve
  kubectl wait --for=condition=Available deployment/kserve-controller-manager -n kserve --timeout=120s

  echo "📄  Current inferenceservice-config ConfigMap:"
  kubectl get configmap inferenceservice-config -n kserve -o yaml
fi



# ────────────────────────────── 4. Gateway + Routes ──────────────────────────
echo "🔧 4. Setting up Gateway and domain-based routes"
kubectl apply -f 02-gateway-configuration.yaml
kubectl apply -f 03-model-routing-domains.yaml

if [[ "$OCP" == true ]]; then
  echo "Getting Base Domain"
  BASE_DOMAIN=$(oc whoami --show-server | sed 's/https:\/\/api\.//' | sed 's/:.*//')
  kubectl patch -n llm Gateway inference-gateway --type=json -p='[{"op": "replace", "path": "/spec/listeners/0/hostname", "value": "*.'${BASE_DOMAIN}'"}]'
  kubectl patch -n llm HTTPRoute simulator-domain-route --type=json -p='[{"op": "replace", "path": "/spec/hostnames/0", "value": "simulator-llm.apps.'${BASE_DOMAIN}'"}]'
fi

if [[ -x ./setup-local-domains.sh ]]; then
  ./setup-local-domains.sh setup
fi

# ────────────────────────────── 5. Kuadrant Operator ─────────────────────────
if [[ "$OCP" == true ]]; then
  echo "🔧 Installing Openshift Connectivity Link operator"
  kubectl apply -k kustomize/connectivitylink/

  echo "⏳   Waiting for Connectivity Link operator"
  kubectl wait --for=condition=Available deployment/kuadrant-operator-controller-manager -n kuadrant-system --timeout=300s

  echo "🔧 Installing Kuadrant Instance"
  kubectl apply -f 04-kuadrant-operator.yaml
else
  echo "🔧 5. Installing Kuadrant operator"
  helm repo add kuadrant https://kuadrant.io/helm-charts
  helm repo update

  helm install kuadrant-operator kuadrant/kuadrant-operator \
    --create-namespace \
    --namespace kuadrant-system

  kubectl apply -f 04-kuadrant-operator.yaml

  echo "⏳   Waiting for Kuadrant operator"
  kubectl wait --for=condition=Available deployment/kuadrant-operator-controller-manager -n kuadrant-system --timeout=300s
fi
# ────────────────────────────── 6. Model deployment ──────────────────────────
echo "🔧 6. Deploying model(s)"

case "$MODEL_TYPE" in
  simulator)
    if [[ "$OCP" == true ]]; then
      kubectl apply -f ../model_serving/vllm-simulator-kserve-openshift.yaml
    else
      kubectl apply -f ../model_serving/vllm-simulator-kserve.yaml
    fi
    kubectl wait --for=condition=Ready inferenceservice/vllm-simulator -n "$NAMESPACE" --timeout=120s
    ;;
  qwen3)
    if [[ "$OCP" == true ]]; then
      kubectl apply -f ../model_serving/qwen3-0.6b-vllm-raw-openshift.yaml
    else
      kubectl apply -f ../model_serving/qwen3-0.6b-vllm-raw.yaml
    fi
    kubectl apply -f ../model_serving/qwen3-0.6b-vllm-raw.yaml
    # Uncomment the wait if you want the accelerator to finish loading before proceeding
    # kubectl wait --for=condition=Ready inferenceservice/qwen3-0-6b-instruct -n "$NAMESPACE" --timeout=900s
    ;;
  all)
    kubectl apply -f ../model_serving/vllm-latest-runtime.yaml
    kubectl apply -f ../model_serving/vllm-simulator-kserve.yaml
    kubectl apply -f ../model_serving/qwen3-0.6b-vllm-raw.yaml
    kubectl wait --for=condition=Ready inferenceservice/vllm-simulator       -n "$NAMESPACE" --timeout=120s
    kubectl wait --for=condition=Ready inferenceservice/qwen3-0-6b-instruct  -n "$NAMESPACE" --timeout=900s
    ;;
esac

# ────────────────────────────── 7. Gateway policies ──────────────────────────
echo "🔧 7. Applying API-key auth & rate-limit policies"
kubectl apply -f 05-api-key-secrets.yaml
kubectl apply -f 06-auth-policies-apikey.yaml
kubectl apply -f 07-rate-limit-policies.yaml
kubectl rollout restart deployment kuadrant-operator-controller-manager -n kuadrant-system
kubectl wait --for=condition=Available deployment/kuadrant-operator-controller-manager -n kuadrant-system --timeout=300s

# ────────────────────────────── 8. Observability ─────────────────────────────
if [[ "$SKIP_METRICS" == false ]]; then
  echo "🔧 8. Installing Prometheus observability"
  
  # Install Prometheus Operator
  kubectl apply --server-side --field-manager=quickstart-installer -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/master/bundle.yaml
  
  # Wait for Prometheus Operator to be ready
  kubectl wait --for=condition=Available deployment/prometheus-operator -n default --timeout=300s
  
  # From models-aas/deployment/kuadrant Kuadrant prometheus observability
  kubectl apply -k kustomize/prometheus/
else
  echo "⏭️  8. Skipping Prometheus observability (--skip-metrics flag)"
fi

# ────────────────────────────── 9. Verification ──────────────────────────────
echo "🔧 9. Verifying objects"
kubectl get gateway,httproute,authpolicy,ratelimitpolicy -n "$NAMESPACE"
kubectl get inferenceservice,pods -n "$NAMESPACE"

echo
echo "✅ MaaS installation complete!"
echo
echo "🔌 Port-forward the gateway in a separate terminal:"
echo "   kubectl port-forward -n $NAMESPACE svc/inference-gateway-istio 8000:80"
echo

if [[ "$SKIP_METRICS" == false ]]; then
echo "📊 Access Prometheus metrics (in separate terminals):"
echo "   kubectl port-forward -n llm-observability svc/models-aas-observability 9090:9090"
echo "   kubectl port-forward -n kuadrant-system svc/limitador-limitador 8080:8080"
echo "   Then visit: http://localhost:9090 (Prometheus) and http://localhost:8080/metrics (Limitador)"
echo
fi
echo "🎯 Test examples (domain routing):"

if [[ "$MODEL_TYPE" == "simulator" || "$MODEL_TYPE" == "all" ]]; then
cat <<'EOF'
# Free tier (5 req/2 min) – Simulator
curl -H 'Authorization: APIKEY freeuser1_key' \
     -H 'Content-Type: application/json' \
     -d '{"model":"simulator-model","messages":[{"role":"user","content":"Hello from free tier!"}]}' \
     http://simulator.maas.local:8000/v1/chat/completions
EOF
echo
fi

if [[ "$MODEL_TYPE" == "qwen3" || "$MODEL_TYPE" == "all" ]]; then
cat <<'EOF'
# Premium tier – Qwen3-0.6 B
curl -H 'Authorization: APIKEY premiumuser1_key' \
     -H 'Content-Type: application/json' \
     -d '{"model":"qwen3-0-6b-instruct","messages":[{"role":"user","content":"Hello! Write a Python function."}]}' \
     http://qwen3.maas.local:8000/v1/chat/completions
EOF
echo
fi

cat <<'EOF'
# Un-authenticated request (should be blocked)
timeout 5 curl -H 'Content-Type: application/json' \
        -d '{"model":"simulator-model","messages":[{"role":"user","content":"Hello"}]}' \
        http://simulator.maas.local:8000/v1/chat/completions
EOF

echo
echo "📊  Available API keys"
echo "    Free:      freeuser1_key, freeuser2_key (5 req/2 min)"
echo "    Premium:   premiumuser1_key, premiumuser2_key (20 req/2 min)"
echo "    Forward the inference gateway with → kubectl port-forward -n llm svc/inference-gateway-istio 8000:80"
echo "    🤖 Run an automated quota stress with → scripts/test-request-limits.sh"
echo
echo "🔥 Deploy complete"

if [[ "$OCP" == true ]]; then
  echo "⚠️ Please validate that you created your `wasm-plugin-pull-secret` pull secret inside of $NAMESPACE"
  echo "More info here: https://developers.redhat.com/products/red-hat-connectivity-link/quick-setup"
fi

