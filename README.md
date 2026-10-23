# Envoy Gateway Zero-Downtime Rollout Controller (EGRC)

**English** | [한국어 (Korean)](./README.ko.md)

[![Kubernetes](https://img.shields.io/badge/Kubernetes-v1.30%2B-blue?logo=kubernetes)](https://kubernetes.io/)
[![Envoy Gateway](https://img.shields.io/badge/Envoy%20Gateway-v1.6%2B-purple?logo=envoyproxy)](https://gateway.envoyproxy.io/)
[![ArgoCD Ready](https://img.shields.io/badge/ArgoCD-GitOps%20Compatible-orange?logo=argo)](https://argoproj.github.io/cd/)
[![Docker Image](https://img.shields.io/badge/Docker%20Hub-premisan0%2Fenvoy--gateway--rollout--controller-blue?logo=docker)](https://hub.docker.com/r/premisan0/envoy-gateway-rollout-controller)

**EGRC (Envoy Gateway Rollout Controller)** is a dedicated Kubernetes canary rollout operator and real-time dashboard platform designed to achieve **zero downtime** during Envoy Gateway control plane and data plane (DaemonSet) upgrades.

---

## 1. Background & Problem Statement

### As-Is (Traditional Operation)
* Upgrading the Envoy Gateway control plane triggers simultaneous rolling updates of the Envoy Proxy DaemonSet across all worker nodes, causing connection drops, RST packets, and brief outages.
* To avoid disruption, infrastructure teams deploy duplicate Envoy Gateway installations across separate node pools (e.g., `zone-a`, `zone-b`) and multiple namespaces, manually toggling DNS or external balancers—an error-prone, resource-heavy practice.

### To-Be (With EGRC)
* **Single Namespace Management**: Deploy and operate a single Envoy Gateway stack in `envoy-gateway-system` while guaranteeing 0% packet loss during upgrades.
* **Progressive Node-Level Canary**: Incrementally deploy shadow/canary pods across ingress worker node pools and verify xDS synchronization.
* **Envoy Admin API Graceful Drain**: Directly calls Envoy's Admin API (`/drain_listeners?inboundonly`) rather than relying on blunt Kubernetes container termination, preserving in-flight sessions while shifting new connections smoothly.
* **Automated Ephemeral Prometheus**: Spawns an on-demand, lightweight Prometheus instance strictly for the upgrade window to scrape high-resolution metrics every 5 seconds, automatically tearing it down once rollout completes (Zero external dependency).
* **GitOps & ArgoCD Ready**: Declare a single custom resource (`EnvoyGatewayRollout`) as an ArgoCD Application, and all underlying Gateway API resources (`Gateway`, `EnvoyProxy`, `GatewayClass`) are automatically tracked and reconciled.
* **Built-in Real-Time Web Dashboard**: Single container binary offering node topology grid visualization, live SSE metric streaming, manual controls (pause, resume, force promote, abort), and bilingual support (English / 한국어).

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
│  - Real-Time PromQL Charts (5xx Rate, Latency, Errors) │
│  - Controls: [Pause] [Resume] [Force Promote] [Abort]  │
└───────────────┬────────────────────────┬───────────────┘
                │                        │
       (PromQL 15s Query)         (OwnerReference Child Objects)
                ▼                        ▼
┌──────────────────────────┐    ┌─────────────────────────────────┐
│   Ephemeral Prometheus   │    │     Child Resources in ArgoCD   │
│ (Active only on rollout) │    │ - GatewayClass (eg)             │
│ - Envoy 5xx Error (<1%)  │    │ - EnvoyProxy (proxy-config)     │
│ - Upstream Conn Errors   │    │ - Gateway (eg - Port 80, 443)   │
└──────────────────────────┘    └─────────────────┬───────────────┘
                                                  │
                                                  ▼
┌─────────────────────────────────────────────────────────────────┐
│              Kubernetes Ingress LB Worker Nodes                 │
│                                                                 │
│  [Node 1: Canary]            [Node 2: Stable]   [Node 3: Stable]│
│   ├── Envoy v1.9.2 (Active)   └── Envoy v1.8.5   └── Envoy v1.8.5│
│   └── Envoy v1.8.5 (Draining)                                   │
└─────────────────────────────────────────────────────────────────┘
```

---

## 3. Declarative Specification (CRD: `EnvoyGatewayRollout`)

Short name: **`egrc`** (`kubectl get egrc -n envoy-gateway-system`)

```yaml
apiVersion: rollout.envoyproxy.io/v1alpha1
kind: EnvoyGatewayRollout
metadata:
  name: eg
  namespace: envoy-gateway-system
spec:
  # Target Envoy Gateway release version (e.g., v1.8.5 -> v1.9.2)
  version: "v1.8.5"

  # Single target namespace replacing multi-namespace setups
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

  # Progressive node-level canary strategy
  strategy:
    steps:
      - nodes: "1"          # Step 1: 1 canary node
        pauseDuration: "3m" # Metric observation duration
      - nodes: "3"          # Step 2: 3 canary nodes
        pauseDuration: "5m"
      - nodes: "100%"       # Step 3: Remaining nodes
        pauseDuration: "5m"

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
          sum(rate(envoy_cluster_upstream_rq_xx{instance=~"$CANARY_NODES.*"}[1m])) * 100
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

## 4. GitOps & ArgoCD Integration

### 4.1 Custom Lua Health Check (`argocd-cm`)
Renders standard ArgoCD health badges (`Healthy`, `Progressing`, `Suspended`, `Degraded`) showing step progression and remaining observation countdown:

```bash
kubectl patch cm argocd-cm -n argocd --patch-file config/argocd/argocd-cm-patch.yaml
```

### 4.2 Prevent UI Control Conflicts (`ignoreDifferences`)
Ensures manual operator actions (pause, force promote, instant abort) via the Web UI are not overridden by ArgoCD automated self-healing:

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

## 5. Built-in Web Dashboard

The controller embeds a lightweight React frontend served on port 8080:

* **Access**: `http://<controller-ip-or-ingress>:8080`
* **Features**:
  1. **Node Topology View**: Real-time colored cards (Stable, Canary, Draining, Completed, Aborted) with live connection counters and drain progress.
  2. **Prometheus Metric Analyzer**: Real-time gauges for 5xx error rate (with threshold boundary lines), connection errors, and P99 latency.
  3. **Action Controls**:
     * `[Upgrade Envoy Version]`: Select official Helm OCI releases (e.g. `v1.9.2`) to trigger canary upgrades.
     * `[Pause / Resume]`: Pause and resume metric evaluation timers.
     * `[Force Promote]`: Skip waiting period and advance immediately to the next node phase.
     * `[Manual Rollback]`: Instant zero-downtime rollback to the previous stable release.
  4. **Internationalization (i18n)**: One-click toggle between **English** (default) and **한국어**.

---

## 6. Getting Started & Installation

### 6.1 Local Build
```bash
# Build frontend UI and Go binary
make build

# Run unit tests
make test
```

### 6.2 Docker Image
Published official container image:
```bash
docker pull premisan0/envoy-gateway-rollout-controller:latest
```

### 6.3 Deploy to Kubernetes
```bash
# 1. Register CRD
kubectl apply -f config/crd/rollout.envoyproxy.io_envoygatewayrollouts.yaml

# 2. Deploy RBAC and Controller Manager
kubectl apply -f config/rbac/service_account.yaml
kubectl apply -f config/rbac/role.yaml
kubectl apply -f config/rbac/role_binding.yaml
kubectl apply -f config/manager/deployment.yaml
kubectl apply -f config/manager/service.yaml

# 3. Create Sample Rollout Resource
kubectl apply -f config/samples/envoy-gateway/tls_secret.yaml
kubectl apply -f config/samples/envoy-gateway/rollout_envoy_gateway_system.yaml

# (Optional) Deploy Sample Workload (Nginx) via ArgoCD
kubectl apply -f config/argocd/application-nginx.yaml

# 4. Check Status (Shortname: egrc)
kubectl get egrc -n envoy-gateway-system
```
