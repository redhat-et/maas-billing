# MaaS Platform - Models as a Service with Policy Management

A comprehensive platform for **Models as a Service** with real-time policy management, built with **Kuadrant**, Istio, Gateway API, and KServe. Features a modern React-based GUI for policy creation, live metrics monitoring, and request simulation.

## 🚀 Features

- **🎯 Policy Management**: Drag-and-drop interface for creating and managing authentication and rate-limiting policies
- **📊 Real-time Metrics**: Live dashboard showing policy enforcement decisions with filtering and analytics
- **🧪 Request Simulation**: Test policies before deployment with comprehensive simulation tools
- **🔐 Authentication**: API key-based auth with team-based access control
- **⚡ Rate Limiting**: Configurable request quotas with time-based restrictions
- **📈 Observability**: Prometheus metrics and real-time monitoring
- **🌐 Domain Routing**: Model-specific subdomains for clean API organization

## 🏗️ Architecture

### Backend Components
- **Model Serving**: KServe-based AI model deployment with vLLM runtime
- **API Gateway**: Istio/Envoy with Gateway API support and Kuadrant integration
- **Policy Engine**: Real-time policy enforcement through Kuadrant (Authorino + Limitador)
- **Metrics Collection**: Live data from Kuadrant components

### Frontend Components  
- **Policy Manager**: Create, edit, and manage policies with intuitive drag-and-drop interface
- **Live Metrics Dashboard**: Real-time view of policy enforcement with filtering capabilities
- **Request Simulator**: Test policies against simulated traffic patterns

## 📋 Prerequisites

- **Kubernetes cluster** (1.25+) with kubectl access
- **Node.js** (18+) and npm
- **Docker** (for local development)

## 🚀 Quick Start

- **For deploying Kuadrant on OpenShift, see → [deployment/kuadrant-openshift](deployment/kuadrant-openshift)**
- **For manual deployment steps of Kuadrant on Vanilla Kube, see → [deployment/kuadrant-openshift](deployment/kuadrant)**
- **For chargeback and token rate limiting see the [demo and deploy instructions](deployment/kuadrant-openshift/README-token-rate-limiting-openshift.md)**

### 1. Deploy Kuadrant Infrastructure (Dev on vanilla kubernetes)

First, deploy the Kuadrant infrastructure that provides the policy enforcement engine:

```bash
git clone https://github.com/redhat-et/maas-billing.git
cd maas-billing/deployment/kuadrant

# Set up local domains (adds entries to /etc/hosts)
./setup-local-domains.sh

# Deploy with simulator (no GPU required)
./install.sh --simulator

# OR deploy with real model (GPU required)  
./install.sh --qwen3

# OR deploy everything including kind cluster
./install.sh --deploy-kind
```

**Available Models:**
- **Simulator**: `http://simulator.maas.local:8000/v1/...` (for testing)
- **Qwen3**: `http://qwen3.maas.local:8000/v1/...` (real model)

### 2. Start the MaaS Platform

After Kuadrant is deployed, start the frontend and backend:

#### Option A: One-Command Start (Recommended)
```bash
# From the repository root
./start-dev.sh
```

This will:
- ✅ Check prerequisites (Kuadrant deployment)
- 🔧 Start backend API server on http://localhost:3001
- 🎨 Start frontend UI on http://localhost:3000
- 📊 Provide monitoring and logging

#### Option B: Manual Start
```bash
# Terminal 1: Start Backend
./start-backend.sh

# Terminal 2: Start Frontend  
./start-frontend.sh
```

### 3. Access the Platform

- **🌐 Frontend UI**: http://localhost:3000
- **🔧 Backend API**: http://localhost:3001
- **📊 API Health**: http://localhost:3001/health
- **📈 Live Metrics**: http://localhost:3001/api/v1/metrics/live-requests

## 🖥️ Using the Platform

### Policy Manager
1. Navigate to **Policy Manager** in the sidebar
2. Click **Create Policy** to open the policy builder
3. Use drag-and-drop to add teams and models
4. Configure rate limits and time restrictions
5. Save to apply policies to Kuadrant

### Live Metrics Dashboard
1. Go to **Live Metrics** to see real-time enforcement
2. Filter by decision type (Accept/Reject) or policy type
3. View detailed policy enforcement reasons
4. Monitor request patterns and policy effectiveness

### Request Simulator
1. Access **Request Simulator** to test policies
2. Select team, model, and configure request parameters
3. Run simulations to see how policies would handle traffic
4. Validate policy configurations before deployment

## 🔧 Development

### Project Structure
```
maas-billing/
├── apps/
│   ├── frontend/          # React frontend with Material-UI
│   │   ├── src/
│   │   │   ├── components/    # Policy Manager, Metrics Dashboard, etc.
│   │   │   ├── hooks/         # API integration hooks
│   │   │   └── services/      # API client
│   │   └── package.json
│   └── backend/           # Node.js/Express API server
│       ├── src/
│       │   ├── routes/        # API endpoints
│       │   ├── services/      # Kuadrant integration
│       │   └── utils/         # Logging and utilities
│       └── package.json
├── deployment/kuadrant/   # Kuadrant infrastructure
└── start-*.sh           # Development scripts
```

### API Endpoints
- `GET /api/v1/policies` - List all policies
- `POST /api/v1/policies` - Create new policy
- `PUT /api/v1/policies/:id` - Update policy
- `DELETE /api/v1/policies/:id` - Delete policy
- `GET /api/v1/metrics/live-requests` - Real-time metrics
- `GET /api/v1/metrics/dashboard` - Dashboard statistics

### Environment Variables
```bash
# Backend (.env)
PORT=3001
FRONTEND_URL=http://localhost:3000
```

## 🛑 Stopping the Platform

```bash
# Stop all services
./stop-dev.sh

# Or manually stop individual components
pkill -f "npm start"    # Stop frontend
pkill -f "npm run dev"  # Stop backend
```

## 📊 Monitoring & Logs

### Application Logs
```bash
# Real-time logs
tail -f backend.log     # Backend API logs
tail -f frontend.log    # Frontend build logs

# Service logs
kubectl logs -n kuadrant-system -l app=limitador
kubectl logs -n kuadrant-system -l app=authorino
```

### Metrics and Health Checks
- Backend health: `curl http://localhost:3001/health`
- Kuadrant status: `kubectl get pods -n kuadrant-system`
- Live metrics: `curl http://localhost:3001/api/v1/metrics/live-requests`

## 🔍 Troubleshooting

### Common Issues

**Port Already in Use**
```bash
# Kill processes on ports 3000/3001
lsof -ti:3000 | xargs kill -9
lsof -ti:3001 | xargs kill -9
```

**Kuadrant Not Ready**
```bash
# Check Kuadrant deployment
kubectl get pods -n kuadrant-system
kubectl get gateways -A
```

**Frontend Not Loading**
```bash
# Clear browser cache and restart frontend
rm -rf apps/frontend/node_modules/.cache
./start-frontend.sh
```

**No Metrics Data**
```bash
# Check Kuadrant components
kubectl port-forward -n kuadrant-system svc/limitador 8080:8080
curl http://localhost:8080/metrics
```

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/amazing-feature`
3. Commit changes: `git commit -m 'Add amazing feature'`
4. Push to branch: `git push origin feature/amazing-feature`
5. Open a Pull Request

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## 📚 Additional Resources

- **Kuadrant Documentation**: https://kuadrant.io/
- **KServe Documentation**: https://kserve.github.io/website/
- **Istio Gateway API**: https://istio.io/latest/docs/tasks/traffic-management/ingress/gateway-api/
