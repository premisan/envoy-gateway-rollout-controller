# Envoy Gateway Zero-Downtime Rollout Controller (EGRC)

[English](./README.md) | **한국어**

[![Kubernetes](https://img.shields.io/badge/Kubernetes-v1.30%2B-blue?logo=kubernetes)](https://kubernetes.io/)
[![Envoy Gateway](https://img.shields.io/badge/Envoy%20Gateway-v1.6%2B-purple?logo=envoyproxy)](https://gateway.envoyproxy.io/)
[![ArgoCD Ready](https://img.shields.io/badge/ArgoCD-GitOps%20Compatible-orange?logo=argo)](https://argoproj.github.io/cd/)
[![Docker Image](https://img.shields.io/badge/Docker%20Hub-premisan0%2Fenvoy--gateway--rollout--controller-blue?logo=docker)](https://hub.docker.com/r/premisan0/envoy-gateway-rollout-controller)

**EGRC(Envoy Gateway Rollout Controller)**는 Kubernetes 환경에서 **Envoy Gateway의 컨트롤 플레인 및 데이터 플레인(DaemonSet) 업그레이드 시 서비스 무중단(Zero Downtime)**을 달성하기 위한 전용 카나리 롤아웃 오퍼레이터 & 실시간 대시보드 플랫폼입니다.

---

## 1. 배경 및 해결 과제 (Problem & Solution)

### As-Is (기존 운영 방식)
* Envoy Gateway 컨트롤 플레인을 업그레이드하면 데이터 플레인(Envoy Proxy DaemonSet) 파드가 전체 노드에서 동시에 롤링 업데이트되어 커넥션 끊김 및 서비스 다운타임 발생.
* 이를 피하기 위해 복수의 인그레스 노드 그룹(예: `zone-a`, `zone-b`) 및 여러 네임스페이스에 양벌로 분리 배포하여 수동으로 한 벌씩 교체하는 비효율적 운영.

### To-Be (본 프로젝트 적용 후)
* **단일 네임스페이스(`envoy-gateway-system`)에 한 벌의 Envoy Gateway만 배포/관리**하면서도 서비스 중단 0% 달성.
* **노드 단위 카나리 롤아웃**: 인그레스 LB 노드 풀에 순차적으로 카나리 파드를 배치하고 xDS 동기화 검증.
* **Envoy Admin API Graceful Drain**: K8s의 단순 종료 대신 Envoy Admin API(`/drain_listeners?inboundonly`)를 직접 호출하여 진행 중인 기존 세션은 온전히 보존하고 신규 트래픽만 신규 버전으로 절체.
* **임시 프로메테우스 (Ephemeral Prometheus) 자동 수명주기**: 업그레이드 시점에만 임시 경량 프로메테우스를 클러스터에 기동하여 5초 주기로 정밀 스크랩 후, 롤아웃 완료 시 자동 삭제(Zero external dependency).
* **ArgoCD 완벽 호환**: 단일 상위 CRD(`EnvoyGatewayRollout`)를 ArgoCD Application으로 등록하면 하위 종속 리소스(Gateway, EnvoyProxy, GatewayClass 등)가 자동으로 추적/관리됨.
* **실시간 웹 UI 대시보드 내장**: 단일 컨테이너 바이너리로 노드 토폴로지 그리드 뷰, 실시간 메트릭 스트리밍, 수동 제어(일시정지, 재개, 즉시 롤백, 강제 승격) 및 다국어(English / 한국어) 지원.

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
│  - 실시간 메트릭 차트 (5xx 에러율, 레이턴시, 커넥션)   │
│  - 수동 제어: [일시정지] [재개] [강제승격] [즉시 롤백] │
└───────────────┬────────────────────────┬───────────────┘
                │                        │
       (PromQL 15s 평가)          (OwnerReference 자식 리소스)
                ▼                        ▼
┌──────────────────────────┐    ┌─────────────────────────────────┐
│   Ephemeral Prometheus   │    │     Child Resources in ArgoCD   │
│ (롤아웃 기간에만 임시 기동) │    │ - GatewayClass (eg)             │
│ - Envoy 5xx 에러율 (<1%) │    │ - EnvoyProxy (proxy-config)     │
│ - Upstream Connection Error│  │ - Gateway (eg - Port 80, 443)   │
└──────────────────────────┘    └─────────────────┬───────────────┘
                                                  │
                                                  ▼
┌─────────────────────────────────────────────────────────────────┐
│              Kubernetes Ingress LB Worker Nodes                 │
│                                                                 │
│  [Node 1: Canary]            [Node 2: Stable]   [Node 3: Stable]│
│   ├── Envoy v1.9.2 (검증)     └── Envoy v1.8.5   └── Envoy v1.8.5│
│   └── Envoy v1.8.5 (Admin Drain)                                │
└─────────────────────────────────────────────────────────────────┘
```

---

## 3. 빠른 시작: 쿠버네티스 클러스터 내부 배포 (단일 커맨드)

EGRC의 모든 구성 요소(Namespace, CRD, RBAC, 컨트롤러 매니저 및 내장 웹 UI 대시보드)를 **단 한 줄의 명령어**로 쿠버네티스 클러스터에 즉시 배포할 수 있습니다:

```bash
# 방법 A: GitHub 원격 매니페스트 기준 단일 커맨드 배포
kubectl apply -f https://raw.githubusercontent.com/premisan/envoy-gateway-rollout-controller/main/deploy/install.yaml

# 방법 B: 로컬 저장소 클론 상태에서 배포
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
웹 브라우저에서 **`http://localhost:8080`**으로 접속하면 실시간 노드 토폴로지 그리드 뷰, 4xx/5xx 메트릭 가드레일 모니터링 및 카나리 제어(일시정지, 재개, 즉시 승급, 롤백) 버튼을 즉시 사용할 수 있습니다.

---

## 4. 선언적 명세 (CRD: `EnvoyGatewayRollout`)

ArgoCD Git 저장소에 아래 매니페스트만 선언해 두면 전체 라이프사이클이 관리됩니다:

```yaml
apiVersion: rollout.envoyproxy.io/v1alpha1
kind: EnvoyGatewayRollout
metadata:
  name: eg
  namespace: envoy-gateway-system
spec:
  # 대상 Envoy Gateway 버전 (예: v1.8.5 -> v1.9.2)
  version: "v1.8.5"

  # 단일 타깃 네임스페이스
  targetNamespace: "envoy-gateway-system"

  # Gateway 리소스 정의 (리스너 및 인증서)
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

  # 데이터 플레인 데몬셋 설정 (노드 그룹 및 Toleration 정의)
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

  # 노드 단위 단계별 카나리 전략
  strategy:
    steps:
      - nodes: "1"          # 1단계: 1개 노드 카나리
        pauseDuration: "3m" # 메트릭 관측 시간
      - nodes: "3"          # 2단계: 3개 노드 카나리
        pauseDuration: "5m"
      - nodes: "100%"       # 3단계: 나머지 전체 노드 일괄 적용
        pauseDuration: "5m"

  # 프로메테우스 분석 및 자동 롤백 기준
  analysis:
    ephemeralPrometheus:
      enabled: true         # 롤아웃 시점에만 임시 프로메테우스 기동
      scrapeInterval: "5s"
      retention: "2h"
    interval: "15s"
    failureLimit: 3         # 연속 3회 임계치 초과 시 자동 롤백
    metrics:
      - name: http-5xx-rate
        threshold: 1.0      # 5xx 에러율 1.0% 초과 시 이상 감지
        query: |
          sum(rate(envoy_cluster_upstream_rq_xx{response_code_class="5", instance=~"$CANARY_NODES.*"}[1m]))
          /
          clamp_min(sum(rate(envoy_cluster_upstream_rq_xx{instance=~"$CANARY_NODES.*"}[1m])), 0.001) * 100
      - name: http-4xx-rate
        threshold: 5.0      # 4xx 에러율 5.0% 초과 시 이상 감지 (Host 헤더 불일치 및 라우팅 경로 매칭 실패 감지)
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

  # 웹 UI에서 수동 제어 플래그 주입 영역
  control:
    paused: false
    abort: false
    forcePromote: false
```

---

## 5. ArgoCD 연동 가이드 (GitOps)

### 5.1 Custom Health Check 등록 (`argocd-cm`)
ArgoCD UI에서 카나리 롤아웃의 진행률, 대기 시간, 단계별 상태가 표준 하트 아이콘(`Healthy`, `Progressing`, `Suspended`, `Degraded`)으로 표시됩니다.

[config/argocd/argocd-cm-patch.yaml](file:///Users/premisan/github/envoy-gateway-rollout-controller/config/argocd/argocd-cm-patch.yaml)을 ArgoCD ConfigMap에 적용합니다:
```bash
kubectl patch cm argocd-cm -n argocd --patch-file config/argocd/argocd-cm-patch.yaml
```

### 5.2 UI 수동 제어와 ArgoCD Self-Heal 충돌 방지 (`ignoreDifferences`)
운영자가 웹 UI에서 긴급하게 [일시정지]나 [수동 롤백]을 눌렀을 때 ArgoCD가 이를 강제로 원복시키는 것을 방지하기 위해 `spec.control`을 무시하도록 설정합니다:
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

컨트롤러 바이너리는 React 프론트엔드가 내장되어 있어 추가적인 웹 서버 없이 바로 브라우저에서 대시보드를 사용할 수 있습니다:

* **접속 주소**: `http://<controller-ip-or-ingress>:8080`
* **주요 기능**:
  1. **Node Topology View**: 노드별 실시간 상태(Stable, Canary, Draining, Upgraded, RolledBack) 색상 타일 및 커넥션 수/드레인 진행률 바 표시.
  2. **Live Prometheus Metric Analyzer**: 5xx 에러율 게이지(임계치 1.0% 점선 마크), Connection Error, P99 레이턴시 스트리밍.
  3. **Action Controls**:
     * `[Upgrade Envoy Version]`: 공식 릴리즈 목록(Helm OCI 차트)에서 대상 버전(예: `v1.9.2`)을 선택하여 노드 카나리 시퀀스 시작.
     * `[Pause / Resume]`: 메트릭 관측 타이머 일시정지 및 재개.
     * `[Force Promote]`: 관측 시간을 건너뛰고 다음 노드군으로 즉시 확장.
     * `[Manual Rollback]`: 즉시 구버전으로 트래픽을 안전 복귀시키는 즉각 롤백.
  4. **다국어 지원 (i18n)**: 상단 헤더의 언어 전환 버튼을 통해 English와 한국어를 자유롭게 전환 가능.

---

## 7. 개발 및 세부 배포 가이드

### 7.1 로컬 빌드
```bash
# Frontend UI 및 Go Controller 빌드
make build

# 단위 테스트 실행
make test
```

### 7.2 Docker 컨테이너 이미지 빌드
```bash
docker pull premisan0/envoy-gateway-rollout-controller:v1.0.9
```

### 7.3 단계별 수동 배포 가이드 (단일 커맨드 대신 개별 매니페스트 확인 시)

`deploy/install.yaml` 단일 커맨드 대신 컴포넌트별 매니페스트를 직접 검토하고 개별 배포할 경우:

#### 1단계: CRD 및 클러스터 RBAC 적용
```bash
# EnvoyGatewayRollout 커스텀 리소스 정의(CRD) 등록
kubectl apply -f config/crd/rollout.envoyproxy.io_envoygatewayrollouts.yaml

# ServiceAccount, ClusterRole, ClusterRoleBinding 생성 (Gateway API CRD 관리 권한 및 EndpointSlice 제어 권한 포함)
kubectl apply -f config/rbac/service_account.yaml
kubectl apply -f config/rbac/role.yaml
kubectl apply -f config/rbac/role_binding.yaml
```

#### 2단계: 컨트롤러 매니저 및 통합 Service 배포
`helm`, `kubectl`, React 웹 UI가 내장된 공식 이미지(`premisan0/envoy-gateway-rollout-controller:v1.0.9`) 기반의 Deployment와 단일 Service(`envoy-gateway-rollout-controller`)를 배포합니다.
```bash
# 컨트롤러 Deployment 배포
kubectl apply -f config/manager/deployment.yaml

# 컨트롤러 Service 배포 (80 -> 컨테이너 8080)
kubectl apply -f config/manager/service.yaml

# 컨트롤러 파드 롤아웃 상태 확인
kubectl rollout status deployment/envoy-gateway-rollout-controller -n envoy-gateway-system
```

#### 3단계: 샘플 Rollout 리소스 생성
```bash
# 샘플 TLS Secret 배포
kubectl apply -f config/samples/envoy-gateway/tls_secret.yaml

# EnvoyGatewayRollout CR 배포
kubectl apply -f config/samples/envoy-gateway/rollout_envoy_gateway_system.yaml

# 실시간 롤아웃 진행 상태 확인 (단축어: egrc)
kubectl get egrc -n envoy-gateway-system -w
kubectl describe egrc eg -n envoy-gateway-system
```

#### 4단계: 컨트롤러 로그 모니터링
```bash
kubectl logs -n envoy-gateway-system -l app.kubernetes.io/name=envoy-gateway-rollout-controller -f
```
