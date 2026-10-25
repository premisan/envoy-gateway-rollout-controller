# Envoy Gateway Rollout Controller (EGRC)

[English](./README.md) | **한국어**

[![Kubernetes](https://img.shields.io/badge/Kubernetes-v1.30%2B-blue?logo=kubernetes)](https://kubernetes.io/)
[![Envoy Gateway](https://img.shields.io/badge/Envoy%20Gateway-v1.6%2B-purple?logo=envoyproxy)](https://gateway.envoyproxy.io/)
[![ArgoCD Ready](https://img.shields.io/badge/ArgoCD-GitOps%20Compatible-orange?logo=argo)](https://argoproj.github.io/cd/)
[![Docker Image](https://img.shields.io/badge/Docker%20Hub-premisan0%2Fenvoy--gateway--rollout--controller-blue?logo=docker)](https://hub.docker.com/r/premisan0/envoy-gateway-rollout-controller)

**EGRC(Envoy Gateway Rollout Controller)는** Kubernetes 환경에서 Envoy Gateway의 컨트롤 플레인 및 데이터 플레인(DaemonSet)을 노드 단위로 점진 배포하고 모니터링하는 카나리 롤아웃 컨트롤러입니다.

---

## 1. 개요 및 주요 기능

### 배경 및 해결 과제
* 기존 방식에서는 Envoy Gateway 컨트롤 플레인을 업그레이드할 때 Envoy Proxy DaemonSet 파드가 전체 노드에서 동시에 롤링 업데이트되어 연결 끊김이나 일시적 오류가 발생할 수 있습니다.
* EGRC는 단일 네임스페이스(`envoy-gateway-system`) 환경에서 인그레스 노드 풀에 순차적으로 카나리 파드를 배치하고 xDS 동기화 및 트래픽 상태를 검증합니다.

### 주요 기능
* **노드 단위 카나리 롤아웃**: 인그레스 노드 풀을 단계별(예: 1대 -> 3대 -> 전체)로 나누어 점진적으로 검증 및 교체합니다.
* **오버래핑 파드 & 커스텀 EndpointSlice**: 온프레미스 CSP L4 LB(`externalTrafficPolicy: Local`) 환경에서도 카나리 파드를 노드에 먼저 기동하고 EndpointSlice로 트래픽을 안전하게 연동합니다.
* **Envoy Admin API Graceful Drain**: K8s의 강제 종료 대신 Envoy Admin API(`/drain_listeners?inboundonly`)를 호출하여 기존 활성 세션을 유지하며 트래픽을 절체합니다.
* **임시 프로메테우스 (Ephemeral Prometheus)**: 롤아웃 기간에만 임시 경량 프로메테우스를 기동하여 5초 주기로 메트릭을 수집하고, 롤아웃 종료 시 자동 정리합니다.
* **메트릭 기반 자동 롤백**: 4xx/5xx 에러율 또는 커넥션 에러가 설정된 임계치를 초과하면 자동으로 카나리를 중단하고 안정 상태로 복귀합니다.
* **ArgoCD GitOps 호환**: 단일 CRD(`EnvoyGatewayRollout`)를 선언하면 하위 리소스(Gateway, EnvoyProxy, GatewayClass)가 자동으로 생성 및 추적됩니다.
* **내장 웹 대시보드**: 단일 컨테이너 바이너리에 React 프론트엔드가 포함되어 있어 노드 토폴로지, 실시간 메트릭 스트리밍, 수동 제어(일시정지, 재개, 즉시 롤백, 강제 승격)를 웹 브라우저에서 바로 사용할 수 있습니다.

---

## 2. 시스템 아키텍처

```
 [운영자 브라우저]
        │
   (SSE 실시간 스트림 / REST API)
        ▼
┌────────────────────────────────────────────────────────┐
│  Rollout Controller & Embedded Dashboard (Port 8080)   │
│  - 노드 토폴로지 그리드 뷰 (Stable / Canary / Drain)    │
│  - 실시간 메트릭 차트 (5xx/4xx 에러율, 커넥션 에러)     │
│  - 수동 제어: [일시정지] [재개] [강제승격] [즉시 롤백] │
└───────────────┬────────────────────────┬───────────────┘
                │                        │
       (PromQL 15s 평가)          (OwnerReference 자식 리소스)
                ▼                        ▼
┌──────────────────────────┐    ┌─────────────────────────────────┐
│   Ephemeral Prometheus   │    │     Child Resources in ArgoCD   │
│ (롤아웃 기간에만 임시 기동) │    │ - GatewayClass (eg)             │
│ - Envoy 5xx/4xx 에러율   │    │ - EnvoyProxy (proxy-config)     │
│ - Upstream Connection Error│  │ - Gateway (eg - Port 80, 443)   │
└──────────────────────────┘    └─────────────────┬───────────────┘
                                                  │
                                                  ▼
┌─────────────────────────────────────────────────────────────────┐
│              Kubernetes Ingress LB Worker Nodes                 │
│                                                                 │
│  [Node 1: Canary]            [Node 2: Stable]   [Node 3: Stable]│
│   ├── Envoy v1.9.1 (Active)   └── Envoy v1.9.0   └── Envoy v1.9.0│
│   └── Envoy v1.9.0 (Admin Drain)                                │
└─────────────────────────────────────────────────────────────────┘
```

---

## 3. 빠른 시작: 클러스터 내부 배포 (단일 커맨드)

EGRC의 모든 구성 요소(CRD, RBAC, 컨트롤러 매니저 및 내장 웹 UI)를 기존 `envoy-gateway-system` 네임스페이스에 단일 명령어로 배포할 수 있습니다:

```bash
# 원격 GitHub 매니페스트 직접 배포
kubectl apply -f https://raw.githubusercontent.com/premisan/envoy-gateway-rollout-controller/main/deploy/install.yaml

# 또는 로컬 저장소 클론 상태에서 배포
kubectl apply -f deploy/install.yaml
```

컨트롤러 파드가 정상 준비될 때까지 대기합니다:
```bash
kubectl rollout status deployment/envoy-gateway-rollout-controller -n envoy-gateway-system
```

### 웹 UI 대시보드 접속
컨트롤러 Service를 로컬 포트로 포트포워딩하여 브라우저에서 대시보드에 접속합니다:
```bash
kubectl port-forward -n envoy-gateway-system svc/envoy-gateway-rollout-controller 8080:80
```
브라우저에서 **[http://localhost:8080](http://localhost:8080)** 으로 접속하면 실시간 노드 토폴로지, 메트릭 모니터링 및 카나리 제어 기능을 바로 사용할 수 있습니다.

---

## 4. 선언적 명세 (CRD: `EnvoyGatewayRollout`)

단축어: **`egr`**, **`egrollout`** (`kubectl get egr -n envoy-gateway-system`)

```yaml
apiVersion: rollout.envoyproxy.io/v1alpha1
kind: EnvoyGatewayRollout
metadata:
  name: eg
  namespace: envoy-gateway-system
spec:
  # 대상 Envoy Gateway 버전 (예: v1.8.5 -> v1.9.1)
  version: "v1.9.0"

  # 타깃 네임스페이스
  targetNamespace: "envoy-gateway-system"

  # Gateway 리소스 정의
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

  # 데이터 플레인 데몬셋 설정
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

  # 노드 단위 단계별 카나리 전략 (Argo Rollouts 스타일 pause 지원)
  strategy:
    steps:
      - nodes: "1"          # 1단계: 1개 노드 카나리 배포
        pause: {}           # 수동 승급 전까지 영구 대기 (pause: {} 또는 pauseDuration: "3m" 시간 지정 대기)
      - nodes: "3"          # 2단계: 3개 노드 카나리 배포
        pause:
          duration: "5m"    # 5분 관측 후 자동 진행
      - nodes: "100%"       # 3단계: 나머지 전체 노드 적용
        pause:
          duration: "5m"

  # 메트릭 분석 및 자동 롤백 기준
  analysis:
    ephemeralPrometheus:
      enabled: true         # 롤아웃 시점에만 임시 프로메테우스 기동
      scrapeInterval: "5s"
      retention: "2h"
    interval: "15s"
    failureLimit: 3         # 연속 3회 초과 시 자동 롤백
    metrics:
      - name: http-5xx-rate
        threshold: 1.0      # 5xx 에러율 1.0% 초과 시 감지
        query: |
          sum(rate(envoy_cluster_upstream_rq_xx{response_code_class="5", instance=~"$CANARY_NODES.*"}[1m]))
          /
          clamp_min(sum(rate(envoy_cluster_upstream_rq_xx{instance=~"$CANARY_NODES.*"}[1m])), 0.001) * 100
      - name: http-4xx-rate
        threshold: 5.0      # 4xx 에러율 5.0% 초과 시 감지 (라우팅 오류 등)
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

  # 웹 UI 수동 제어 필드
  control:
    paused: false
    abort: false
    forcePromote: false
```

---

## 5. ArgoCD 연동 가이드 (GitOps)

### 5.1 Custom Health Check 등록 (`argocd-cm`)
ArgoCD UI에서 카나리 롤아웃 진행률과 대기 시간을 표준 헬스 상태(`Healthy`, `Progressing`, `Suspended`, `Degraded`)로 표시합니다:

```bash
kubectl patch cm argocd-cm -n argocd --patch-file config/argocd/argocd-cm-patch.yaml
```

### 5.2 UI 수동 제어와 ArgoCD 충돌 방지 (`ignoreDifferences`)
운영자가 웹 UI에서 일시정지나 롤백을 수행할 때 ArgoCD Self-Heal과의 충돌을 방지하기 위해 `spec.control`을 무시하도록 설정합니다:

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

## 6. 실시간 웹 UI 대시보드

컨트롤러 바이너리에 내장된 경량 웹 대시보드를 통해 실시간 제어가 가능합니다:

* **접속 주소**: `http://<controller-ip-or-ingress>:8080`
* **주요 기능**:
  1. **노드 토폴로지 뷰**: 노드별 실시간 상태(Stable, Canary, Draining, Upgraded, RolledBack), 실제 활성 세션 수, 드레인 진행률 바 표시.
  2. **프로메테우스 메트릭 차트**: 5xx/4xx 에러율 및 커넥션 에러 실시간 게이지 및 임계치 표시.
  3. **제어 버튼**:
     * `[Upgrade Envoy Version]`: 대상 버전 선택 후 카나리 롤아웃 시작.
     * `[Pause / Resume]`: 관측 타이머 일시정지 및 재개.
     * `[Force Promote]`: 관측 시간을 건너뛰고 다음 단계로 승격.
     * `[Manual Rollback]`: 즉시 안정 버전으로 복귀.
  4. **다국어 지원 (i18n)**: English 및 한국어 전환 지원.

---

## 7. 개발 및 세부 배포 가이드

### 7.1 로컬 빌드 및 테스트
```bash
# Frontend UI 및 Go Controller 빌드
make build

# 단위 테스트 실행
make test
```

### 7.2 Docker 이미지
```bash
docker pull premisan0/envoy-gateway-rollout-controller:v1.0.16
```

### 7.3 컴포넌트별 개별 배포 (수동)
`deploy/install.yaml` 대신 개별 리소스를 검토하며 배포할 경우:

```bash
# 1. CRD 및 RBAC 등록
kubectl apply -f config/crd/rollout.envoyproxy.io_envoygatewayrollouts.yaml
kubectl apply -f config/rbac/service_account.yaml
kubectl apply -f config/rbac/role.yaml
kubectl apply -f config/rbac/role_binding.yaml

# 2. 컨트롤러 매니저 및 서비스 배포
kubectl apply -f config/manager/deployment.yaml
kubectl apply -f config/manager/service.yaml

# 3. 샘플 롤아웃 리소스 생성
kubectl apply -f config/samples/envoy-gateway/tls_secret.yaml
kubectl apply -f config/samples/envoy-gateway/rollout_envoy_gateway_system.yaml

# 4. 롤아웃 상태 및 로그 확인
kubectl get egr -n envoy-gateway-system -w
kubectl logs -n envoy-gateway-system -l app.kubernetes.io/name=envoy-gateway-rollout-controller -f
```
