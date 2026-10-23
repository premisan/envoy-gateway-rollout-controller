import React, { createContext, useContext, useState, useEffect } from 'react';

export type Language = 'en' | 'ko';

export const translations = {
  en: {
    // Navbar
    appTitle: 'Envoy Gateway Rollout Controller',
    appSubtitle: 'Node-level Canary Orchestration, Ephemeral Prometheus & Graceful Drain',
    namespace: 'Namespace',
    sseLive: 'SSE Stream Live',
    sseReconnecting: 'Reconnecting...',

    // ControlPanel
    controlPanelTitle: 'Rollout & Safety Controls',
    controlPanelSubtitle: 'Directly manage rollout progress, pause timers, or execute emergency rollbacks',
    btnUpgrade: 'Upgrade Envoy Version',
    btnPause: 'Pause Rollout',
    btnResume: 'Resume Rollout',
    btnForcePromote: 'Force Promote Next Step',
    btnAbort: 'Emergency Rollback',
    confirmAbort: 'Are you sure you want to abort and rollback immediately?',

    // RolloutHeader
    activeVersion: 'Active Version',
    targetVersion: 'Target Version',
    currentStep: 'Current Step',
    timeRemaining: 'Time Remaining',
    canaryProgress: 'Canary Node Ratio',
    ephemeralPrometheus: 'Ephemeral Prometheus',
    statusActive: 'Active',
    statusCompleted: 'Completed',
    statusPaused: 'Paused',
    statusDegraded: 'Degraded',
    statusPending: 'Pending',
    targetGateway: 'Target Gateway',

    // Pipeline
    pipelineTitle: 'Live Rollout Lifecycle Pipeline',
    pipelineSubtitle: 'Detailed stage-by-stage progression of canary deployment, metric analysis, and graceful listener draining',
    step1Title: 'Canary Pod & xDS Sync',
    step1Desc: 'Deploys shadow Envoy Proxy pod and verifies xDS route configuration',
    step2Title: 'Ephemeral Metric Guardrail',
    step2Desc: 'Spawns temporary Prometheus. Evaluates 5xx rate (<1%) & connection errors every 15s',
    step3Title: 'Envoy Admin API Drain',
    step3Desc: 'Calls /drain_listeners?inboundonly to shift traffic while preserving active sessions',
    step4Title: 'Progressive Node Cutover',
    step4Desc: 'Expands canary ratio progressively across worker node pools',
    step5Title: 'Completion & Teardown',
    step5Desc: 'All nodes upgraded. Auto-terminates ephemeral Prometheus and syncs ArgoCD health',
    currentActionDetails: 'Current Execution Details',

    // NodeTopologyView
    nodeTopologyTitle: 'Worker Node Topology Grid',
    nodeTopologySubtitle: 'Live per-node canary deployment, drain progress, and active traffic distribution',
    nodeFilterAll: 'All Nodes',
    nodeFilterCanary: 'Canary',
    nodeFilterStable: 'Stable',
    nodeFilterDraining: 'Draining',
    activeSessions: 'Active Sessions',
    drainProgress: 'Drain Progress',
    versionBadge: 'Version',

    // MetricCharts
    metricAnalysisTitle: 'Automated Prometheus Metric Guardrails',
    metricAnalysisSubtitle: 'Evaluated every 15s via Ephemeral Prometheus. Auto-rollback triggered upon 3 consecutive breaches.',
    metric5xx: 'HTTP 5xx Error Rate',
    metricConnError: 'Upstream Connection Errors',
    metricLatency: 'P99 Upstream Latency',
    threshold: 'Threshold',
    safeZone: 'Safe Range',
    breached: 'Breached',
    healthyMetric: 'Healthy (Normal)',

    // UpgradeModal
    modalTitle: 'Upgrade Envoy Gateway Data Plane',
    modalSubtitle: 'Initiate zero-downtime progressive node-level canary rollout',
    selectVersionLabel: 'Select Target Envoy Gateway Release',
    currentVersionLabel: 'Current Stable Version',
    fetchingReleases: 'Fetching official releases...',
    selectVersionPlaceholder: 'Choose a release version...',
    helmChartPreview: 'Official Helm OCI Chart:',
    btnCancel: 'Cancel',
    btnLaunch: 'Launch Canary to',
    btnLaunching: 'Starting...',
    prereleaseBadge: 'Pre-release',
    latestBadge: 'Latest',

    // Footer
    footerText: 'Envoy Gateway Rollout Controller • envoy-gateway-system',
    noRollouts: 'No Envoy Gateway Rollout resources detected.',
  },
  ko: {
    // Navbar
    appTitle: 'Envoy Gateway Rollout Controller',
    appSubtitle: '노드 단위 카나리 오케스트레이션, 임시 프로메테우스 및 무중단 세션 보존 드레인',
    namespace: '네임스페이스',
    sseLive: 'SSE 실시간 연결됨',
    sseReconnecting: '연결 재시도 중...',

    // ControlPanel
    controlPanelTitle: '롤아웃 제어 및 세이프티 가드',
    controlPanelSubtitle: '롤아웃 진행 상황을 직접 제어하거나 타이머 일시정지, 긴급 롤백을 수행합니다',
    btnUpgrade: 'Envoy Gateway 버전 업그레이드',
    btnPause: '롤아웃 일시정지',
    btnResume: '롤아웃 재개',
    btnForcePromote: '다음 단계로 즉시 승급',
    btnAbort: '즉각 안전 롤백',
    confirmAbort: '정말로 롤아웃을 중단하고 구버전으로 즉시 롤백하시겠습니까?',

    // RolloutHeader
    activeVersion: '현재 운영 버전',
    targetVersion: '목표 업그레이드 버전',
    currentStep: '현재 단계',
    timeRemaining: '단계 관측 잔여 시간',
    canaryProgress: '카나리 노드 비율',
    ephemeralPrometheus: '임시 프로메테우스',
    statusActive: '진행 중',
    statusCompleted: '완료됨',
    statusPaused: '일시정지됨',
    statusDegraded: '이상 감지 / 롤백',
    statusPending: '대기 중',
    targetGateway: '대상 Gateway',

    // Pipeline
    pipelineTitle: '실시간 롤아웃 생애주기 파이프라인',
    pipelineSubtitle: '카나리 파드 배치, 임시 메트릭 분석, 리스너 세션 보존 드레인 및 단계별 절체 진행 현황',
    step1Title: '카나리 파드 배치 & xDS 동기화',
    step1Desc: '인그레스 노드에 카나리 파드를 배치하고 Envoy 컨트롤 플레인과 xDS 경로를 동기화합니다',
    step2Title: '임시 프로메테우스 메트릭 분석',
    step2Desc: '임시 프로메테우스를 기동하여 5초 주기로 5xx 에러율(<1%)과 커넥션 오류를 정밀 평가합니다',
    step3Title: 'Envoy Admin API 리스너 드레인',
    step3Desc: '/drain_listeners?inboundonly를 호출하여 기존 활성 세션을 보존하며 신규 트래픽을 안전 절체합니다',
    step4Title: '점진적 노드 트래픽 승급',
    step4Desc: '설정된 전략(1노드 ➔ 3노드 ➔ 100%)에 따라 전체 워커 노드 풀로 점진적 확장을 수행합니다',
    step5Title: '완료 및 임시 자원 회수',
    step5Desc: '모든 노드 업그레이드 완료. 임시 프로메테우스를 자동 회수하고 ArgoCD Health를 Healthy로 동기화합니다',
    currentActionDetails: '현재 실행 상세',

    // NodeTopologyView
    nodeTopologyTitle: '워커 노드 토폴로지 그리드',
    nodeTopologySubtitle: '노드별 실시간 카나리 배포 상태, 리스너 드레인 진행률 및 활성 트래픽 분포',
    nodeFilterAll: '전체 노드',
    nodeFilterCanary: '카나리',
    nodeFilterStable: '스테이블(운영)',
    nodeFilterDraining: '드레인 진행 중',
    activeSessions: '활성 세션 수',
    drainProgress: '드레인 진행률',
    versionBadge: '버전',

    // MetricCharts
    metricAnalysisTitle: '프로메테우스 자동 롤백 가드레일',
    metricAnalysisSubtitle: '임시 프로메테우스를 통해 15초마다 메트릭을 자동 평가하며, 3회 연속 임계치 초과 시 즉시 자동 롤백됩니다.',
    metric5xx: 'HTTP 5xx 에러율',
    metricConnError: '업스트림 커넥션 오류',
    metricLatency: 'P99 업스트림 레이턴시',
    threshold: '임계치',
    safeZone: '안전 구간',
    breached: '임계치 초과',
    healthyMetric: '정상 (안전)',

    // UpgradeModal
    modalTitle: 'Envoy Gateway 데이터 플레인 업그레이드',
    modalSubtitle: '노드 단위 점진적 카나리 배포를 통해 서비스 무중단 업그레이드를 시작합니다',
    selectVersionLabel: '업그레이드 대상 Envoy Gateway 버전 선택',
    currentVersionLabel: '현재 운영 버전',
    fetchingReleases: '공식 릴리즈 목록 조회 중...',
    selectVersionPlaceholder: '릴리즈 버전을 선택하세요...',
    helmChartPreview: '공식 Helm OCI 차트 주소:',
    btnCancel: '취소',
    btnLaunch: '카나리 배포 시작:',
    btnLaunching: '시작 중...',
    prereleaseBadge: '프리릴리즈',
    latestBadge: '최신 버전',

    // Footer
    footerText: 'Envoy Gateway Rollout Controller • envoy-gateway-system',
    noRollouts: '감지된 Envoy Gateway Rollout 리소스가 없습니다.',
  },
};

export type TranslationKey = keyof typeof translations.en;

interface I18nContextType {
  language: Language;
  setLanguage: (lang: Language) => void;
  t: (key: TranslationKey) => string;
}

const I18nContext = createContext<I18nContextType | undefined>(undefined);

export const I18nProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [language, setLanguageState] = useState<Language>(() => {
    const saved = localStorage.getItem('egrc_lang');
    return (saved === 'ko' || saved === 'en') ? saved : 'en';
  });

  const setLanguage = (lang: Language) => {
    setLanguageState(lang);
    localStorage.setItem('egrc_lang', lang);
  };

  const t = (key: TranslationKey): string => {
    return translations[language][key] || translations.en[key] || key;
  };

  return (
    <I18nContext.Provider value={{ language, setLanguage, t }}>
      {children}
    </I18nContext.Provider>
  );
};

export const useI18n = (): I18nContextType => {
  const context = useContext(I18nContext);
  if (!context) {
    throw new Error('useI18n must be used within an I18nProvider');
  }
  return context;
};
