# Models as a Service Billing

This repository shows how to run AI models as HTTP APIs on Kubernetes.  
The deployment uses **Kuadrant** together with Istio, Gateway API, and KServe to provide policy, authentication, rate-limiting, chargeback and observability. 

>[!NOTE]
> Chargeback is currently a WIP in upstream Kuadrant metrics.

## Kuadrant deployment

Implementation details live under [`deployment/kuadrant/`](deployment/kuadrant/).

This implementation provides:

- **Model Serving**: KServe-based AI model deployment with vLLM runtime
- **API Gateway**: Istio/Envoy with Gateway API support
- **Authentication**: API key-based auth with tiered access levels
- **Rate Limiting**: Request quotas per API key tier
- **Observability**: Prometheus metrics and monitoring
- **Domain Routing**: Model-specific subdomains for clean API organization

### Domain-Based Routing

Models are accessible via subdomain routing, the quickstart example provides:
- Simulator: `http://simulator.maas.local:8000/v1/chat/completions`
- Qwen3: `http://qwen3.maas.local:8000/v1/chat/completions`

## Installation

For step-by-step manual deployment, see [`deployment/kuadrant/README.md`](deployment/kuadrant/README.md).

### Dev Quickstart

- Currently validate on vanilla kube

>[!TIP]
> OCP is a WIP, and will utilize the RHOAI and other native operators to install infrastructure.

```bash
git clone https://github.com/redhat-et/maas-billing.git
cd deployment/kuadrant

# Add domains to /etc/hosts e.g. 
# Models-as-a-Service local domains
# 127.0.0.1    qwen3.maas.local
# 127.0.0.1    simulator.maas.local
# Or automate with:
./setup-local-domains.sh

# Inference simulator on an existing cluster without a GPU
./install.sh --simulator

# Deploy to OCP
# Assumes RHOAI operator is installed
./install.sh --simulator --ocp
# Real model on an existing cluster with a GPU
./install.sh --qwen3

# Both the Qwen model and the simulator on an existing cluster with a GPU
./install.sh --install-all-models

# Spin up a kind cluster with the simulator
./install.sh --deploy-kind
```
