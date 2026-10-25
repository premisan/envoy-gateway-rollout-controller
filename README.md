# Envoy Gateway Rollout Controller (EGRC)

**English** | [한국어 (Korean)](./README.ko.md)

[![Kubernetes](https://img.shields.io/badge/Kubernetes-v1.30%2B-blue?logo=kubernetes)](https://kubernetes.io/)
[![Envoy Gateway](https://img.shields.io/badge/Envoy%20Gateway-v1.6%2B-purple?logo=envoyproxy)](https://gateway.envoyproxy.io/)
[![ArgoCD Ready](https://img.shields.io/badge/ArgoCD-GitOps%20Compatible-orange?logo=argo)](https://argoproj.github.io/cd/)
[![Docker Image](https://img.shields.io/badge/Docker%20Hub-premisan0%2Fenvoy--gateway--rollout--controller-blue?logo=docker)](https://hub.docker.com/r/premisan0/envoy-gateway-rollout-controller)

**EGRC (Envoy Gateway Rollout Controller)** is a Kubernetes operator and real-time dashboard for managing progressive canary rollouts of Envoy Gateway control plane and data plane (DaemonSet) components.

---

## 1. Overview & Key Features

### Background & Problem Statement
* Standard control plane upgrades trigger simultaneous rolling updates across all worker nodes in the Envoy Proxy DaemonSet, which can cause connection drops or transient errors.
* EGRC runs in a single namespace (`envoy-gateway-system`) and progressively rolls out canary pods across ingress worker node pools while verifying xDS synchronization and metric health.

### Key Features
* **Progressive Node-Level Canary**: Incrementally deploy and test proxy pods across worker nodes in configurable steps (e.g., 1 node -> 3 nodes -> 100%).
* **Overlapping Pods & Custom EndpointSlice**: Spawns an overlapping canary pod on the target node and registers it via a custom `EndpointSlice`, ensuring continuous traffic handling even in on-premises L4 LB (`externalTrafficPolicy: Local`) environments.
* **Envoy Admin API Graceful Drain**: Calls Envoy's Admin API (`/drain_listeners?inboundonly`) instead of relying on abrupt pod termination, preserving in-flight sessions while shifting new connections.
* **Ephemeral Prometheus**: Spawns a lightweight, temporary Prometheus instance only during the rollout window to scrape metrics every 5 seconds, automatically cleaning it up once finished.
* **Automated Rollback on Anomaly**: Automatically halts rollout and rolls back to stable state if 5xx/4xx error rates or connection errors exceed defined thresholds.
* **ArgoCD GitOps Compatibility**: Declare a single custom resource (`EnvoyGatewayRollout`) to automatically track and manage child Gateway API resources (`Gateway`, `EnvoyProxy`, `GatewayClass`).
* **Embedded Web Dashboard**: Single container binary offering node topology grid visualization, real-time PromQL metric charts, manual controls (pause, resume, force promote, rollback), and bilingual support (English / 한국어).

---

## 2. System Architecture

```
 [Operator Browser]
        │
   (SSE Live Stream / REST API)
        ▼
┌────────────────────────────────────────────────────────┐
│  Rollout Controller & Embedded Dashboard (Port 8080)   │
│  - Node Topology Grid View (Stable / Canary / Drain)   │
│  - Real-Time PromQL Charts (5xx/4xx Rate, Errors)      │
│  - Controls: [Pause] [Resume] [Force Promote] [Abort]  │
└───────────────┬────────────────────────┬───────────────┘
                │                        │
       (PromQL 15s Query)         (OwnerReference Child Objects)
                ▼                        ▼
┌──────────────────────────┐    ┌─────────────────────────────────┐
│   Ephemeral Prometheus   │    │     Child Resources in ArgoCD   │
│ (Active only on rollout) │    │ - GatewayClass (eg)             │
│ - Envoy 5xx/4xx Errors   │    │ - EnvoyProxy (proxy-config)     │
│ - Upstream Conn Errors   │    │ - Gateway (eg - Port 80, 443)   │
└──────────────────────────┘    └─────────────────┬───────────────┘
                                                  │
                                                  ▼
┌─────────────────────────────────────────────────────────────────┐
│              Kubernetes Ingress LB Worker Nodes                 │
│                                                                 │
│  [Node 1: Canary]            [Node 2: Stable]   [Node 3: Stable]│
│   ├── Envoy v1.9.1 (Active)   └── Envoy v1.9.0   └── Envoy v1.9.0│
│   └── Envoy v1.9.0 (Draining)                                   │
└─────────────────────────────────────────────────────────────────┘
```

---

## 3. Quick Start: Cluster Deployment (Single Command)

Deploy the complete EGRC stack (CRD, RBAC, Controller Manager, and embedded Web Dashboard) into your existing `envoy-gateway-system` namespace with a single command:

```bash
# Option A: Remote deployment directly via GitHub
kubectl apply -f https://raw.githubusercontent.com/premisan/envoy-gateway-rollout-controller/main/deploy/install.yaml

# Option B: Local deployment from cloned repository
kubectl apply -f deploy/install.yaml
```

Wait for the controller deployment to be ready:
```bash
kubectl rollout status deployment/envoy-gateway-rollout-controller -n envoy-gateway-system
```

### Accessing the Web UI Dashboard
Forward port 80 of the controller Service to your local machine:
```bash
kubectl port-forward -n envoy-gateway-system svc/envoy-gateway-rollout-controller 8080:80
```
Open **`http://localhost:8080`** in your browser to access the real-time node topology grid, metric guardrails, and rollout controls.

---

## 4. Declarative Specification (CRD: `EnvoyGatewayRollout`)

Short name: **`egr`**, **`egrollout`** (`kubectl get egr -n envoy-gateway-system`)

```yaml
apiVersion: rollout.envoyproxy.io/v1alpha1
kind: EnvoyGatewayRollout
metadata:
  name: eg
  namespace: envoy-gateway-system
spec:
  # Target Envoy Gateway release version (e.g., v1.8.5 -> v1.9.1)
  version: "v1.9.0"

  # Target namespace
  targetNamespace: "envoy-gateway-system"

  # Gateway resource definition
  gateway:
    name: "eg"
    gatewayClassName: "eg"
    listeners:
      - name: http
        protocol: HTTP
        port: 80
      - name: https
        protocol: HTTPS
        port: 443
        tlsSecretName: "example-tls-cert"

  # DataPlane DaemonSet configuration
  envoyProxy:
    name: "proxy-config"
    nodeSelector:
      node-role.kubernetes.io/ingress: "true"
    tolerations:
      - key: "node-role.kubernetes.io/ingress"
        operator: "Equal"
        value: "true"
        effect: "NoExecute"
    annotations:
      gateway.envoyproxy.io/node-group: "envoy-gateway"

  # Progressive node-level canary strategy (Argo Rollouts style pause support)
  strategy:
    steps:
      - nodes: "1"          # Step 1: 1 canary node
        pause: {}           # Indefinite pause until manually promoted (or pause: {duration: "3m"})
      - nodes: "3"          # Step 2: 3 canary nodes
        pause:
          duration: "5m"    # Timed observation pause
      - nodes: "100%"       # Step 3: Remaining nodes
        pause:
          duration: "5m"

  # Automated health evaluation with temporary Prometheus
  analysis:
    ephemeralPrometheus:
      enabled: true         # Spawns Prometheus pod during rollout
      scrapeInterval: "5s"
      retention: "2h"
    interval: "15s"
    failureLimit: 3         # Auto-rollback after 3 consecutive breaches
    metrics:
      - name: http-5xx-rate
        threshold: 1.0      # Breached if 5xx rate > 1.0%
        query: |
          sum(rate(envoy_cluster_upstream_rq_xx{response_code_class="5", instance=~"$CANARY_NODES.*"}[1m]))
          /
          clamp_min(sum(rate(envoy_cluster_upstream_rq_xx{instance=~"$CANARY_NODES.*"}[1m])), 0.001) * 100
      - name: http-4xx-rate
        threshold: 5.0      # Breached if 4xx rate > 5.0%
        query: |
          (
            sum(rate(envoy_http_downstream_rq_xx{envoy_response_code_class="4", instance=~"$CANARY_NODES.*"}[1m]))
            or
            sum(rate(envoy_cluster_upstream_rq_xx{envoy_response_code_class="4", instance=~"$CANARY_NODES.*"}[1m]))
            or
            sum(rate(envoy_cluster_upstream_rq_xx{response_code_class="4", instance=~"$CANARY_NODES.*"}[1m]))
          )
          /
          clamp_min(
            sum(rate(envoy_http_downstream_rq_xx{instance=~"$CANARY_NODES.*"}[1m]))
            or
            sum(rate(envoy_cluster_upstream_rq_xx{instance=~"$CANARY_NODES.*"}[1m])),
            0.001
          ) * 100
      - name: connection-errors
        threshold: 5.0
        query: |
          sum(rate(envoy_server_total_connections_error{instance=~"$CANARY_NODES.*"}[1m]))

  # Manual operator controls injected via Web UI
  control:
    paused: false
    abort: false
    forcePromote: false
```

---

## 5. GitOps & ArgoCD Integration

### 5.1 Custom Lua Health Check (`argocd-cm`)
Renders standard ArgoCD health badges (`Healthy`, `Progressing`, `Suspended`, `Degraded`) showing step progression and countdown:

```bash
kubectl patch cm argocd-cm -n argocd --patch-file config/argocd/argocd-cm-patch.yaml
```

### 5.2 Prevent UI Control Conflicts (`ignoreDifferences`)
Ensures manual operator actions (pause, force promote, abort) via the Web UI are not overridden by ArgoCD automated self-healing:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: envoy-gateway
  namespace: argocd
spec:
  ignoreDifferences:
    - group: rollout.envoyproxy.io
      kind: EnvoyGatewayRollout
      jsonPointers:
        - /spec/control
```

---

## 6. Built-in Web Dashboard

The controller embeds a lightweight React frontend served on port 8080:

* **Access**: `http://<controller-ip-or-ingress>:8080`
* **Features**:
  1. **Node Topology View**: Real-time colored cards (Stable, Canary, Draining, Upgraded, RolledBack), live active connection count per node, and drain progress.
  2. **Prometheus Metric Analyzer**: Real-time gauges for 5xx/4xx error rates and connection errors with threshold boundary indicators.
  3. **Action Controls**:
     * `[Upgrade Envoy Version]`: Select official Helm OCI releases (e.g., `v1.9.1`) to trigger canary upgrades.
     * `[Pause / Resume]`: Pause and resume metric evaluation timers.
     * `[Force Promote]`: Skip observation duration and advance to the next step.
     * `[Manual Rollback]`: Safely roll back to the previous stable release.
  4. **Internationalization (i18n)**: Toggle between **English** and **한국어**.

---

## 7. Development & Manual Deployment

### 7.1 Local Build & Test
```bash
# Build frontend UI and Go binary
make build

# Run unit tests
make test
```

### 7.2 Docker Image
```bash
docker pull premisan0/envoy-gateway-rollout-controller:v1.0.16
```

### 7.3 Step-by-Step Manual Deployment
If you prefer to inspect and deploy individual manifests manually:

```bash
# 1. Register CRD & RBAC
kubectl apply -f config/crd/rollout.envoyproxy.io_envoygatewayrollouts.yaml
kubectl apply -f config/rbac/service_account.yaml
kubectl apply -f config/rbac/role.yaml
kubectl apply -f config/rbac/role_binding.yaml

# 2. Deploy Controller Manager & Service
kubectl apply -f config/manager/deployment.yaml
kubectl apply -f config/manager/service.yaml

# 3. Create Sample Rollout Resource
kubectl apply -f config/samples/envoy-gateway/tls_secret.yaml
kubectl apply -f config/samples/envoy-gateway/rollout_envoy_gateway_system.yaml

# 4. Check Status and Logs
kubectl get egr -n envoy-gateway-system -w
kubectl logs -n envoy-gateway-system -l app.kubernetes.io/name=envoy-gateway-rollout-controller -f
```
