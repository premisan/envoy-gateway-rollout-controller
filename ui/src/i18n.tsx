import React, { createContext, useContext, useState } from 'react';

export type Language = 'en' | 'ko';

export const translations = {
  en: {
    // Navbar
    appTitle: 'Envoy Gateway Rollout Controller',
    namespace: 'Namespace',
    sseLive: 'Live',
    sseReconnecting: 'Reconnecting...',

    // ControlPanel
    controlPanelTitle: 'Rollout Controls',
    btnUpgrade: 'Upgrade Version',
    btnPause: 'Pause',
    btnResume: 'Resume',
    btnForcePromote: 'Force Promote',
    btnAbort: 'Rollback',
    confirmAbort: 'Confirm Rollback?',

    // RolloutHeader
    activeVersion: 'Active Version',
    targetVersion: 'Target Version',
    currentStep: 'Current Step',
    timeRemaining: 'Remaining Time',
    canaryProgress: 'Canary Ratio',
    ephemeralPrometheus: 'Metrics Scraper',
    statusActive: 'Active',
    statusCompleted: 'Completed',
    statusPaused: 'Paused',
    statusDegraded: 'Degraded',
    statusPending: 'Pending',
    targetGateway: 'Gateway',

    // Pipeline
    pipelineTitle: 'Rollout Pipeline',
    step1Title: 'Canary & xDS',
    step1Desc: 'Deploy shadow pod & sync routes',
    step2Title: 'Health Check',
    step2Desc: 'Evaluate 5xx error rate & connections',
    step3Title: 'Inbound Drain',
    step3Desc: 'Inbound drain via Admin API',
    step4Title: 'Node Cutover',
    step4Desc: 'Progressive traffic switch',
    step5Title: 'Completion',
    step5Desc: 'All nodes updated & cleanup',
    currentActionDetails: 'Current Status',

    // NodeTopologyView
    nodeTopologyTitle: 'Node Status',
    nodeFilterAll: 'All Nodes',
    nodeFilterCanary: 'Canary',
    nodeFilterStable: 'Stable',
    nodeFilterDraining: 'Draining',
    activeSessions: 'Active Sessions',
    drainProgress: 'Drain Progress',
    versionBadge: 'Version',

    // MetricCharts
    metricAnalysisTitle: 'Metrics & Guardrails',
    metric5xx: 'HTTP 5xx Rate',
    metricConnError: 'Connection Errors',
    metricLatency: 'P99 Latency',
    threshold: 'Limit',
    safeZone: 'Safe',
    breached: 'Breached',
    healthyMetric: 'Normal',

    // UpgradeModal
    modalTitle: 'Upgrade Envoy Gateway',
    modalSubtitle: 'Select a target version to begin rollout',
    selectVersionLabel: 'Target Release Version',
    currentVersionLabel: 'Current Version',
    fetchingReleases: 'Fetching releases...',
    selectVersionPlaceholder: 'Choose version...',
    helmChartPreview: 'Helm OCI Chart:',
    btnCancel: 'Cancel',
    btnLaunch: 'Start Upgrade to',
    btnLaunching: 'Starting...',
    prereleaseBadge: 'Pre-release',
    latestBadge: 'Latest',

    // Footer
    footerText: 'Envoy Gateway Rollout Controller',
    noRollouts: 'No active rollout resources found.',
  },
  ko: {
    // Navbar
    appTitle: 'Envoy Gateway Rollout Controller',
    namespace: '네임스페이스',
    sseLive: '실시간 연결됨',
    sseReconnecting: '연결 중...',

    // ControlPanel
    controlPanelTitle: '롤아웃 제어',
    btnUpgrade: '버전 업그레이드',
    btnPause: '일시정지',
    btnResume: '재개',
    btnForcePromote: '즉시 승급',
    btnAbort: '롤백',
    confirmAbort: '롤백하시겠습니까?',

    // RolloutHeader
    activeVersion: '현재 버전',
    targetVersion: '목표 버전',
    currentStep: '현재 단계',
    timeRemaining: '잔여 시간',
    canaryProgress: '카나리 비율',
    ephemeralPrometheus: '메트릭 수집기',
    statusActive: '진행 중',
    statusCompleted: '완료됨',
    statusPaused: '일시정지됨',
    statusDegraded: '이상 감지',
    statusPending: '대기 중',
    targetGateway: '게이트웨이',

    // Pipeline
    pipelineTitle: '진행 파이프라인',
    step1Title: '카나리 & xDS',
    step1Desc: '파드 배치 및 경로 동기화',
    step2Title: '메트릭 검증',
    step2Desc: '5xx 에러율 및 커넥션 평가',
    step3Title: '인바운드 드레인',
    step3Desc: 'Admin API 인바운드 드레인',
    step4Title: '노드 절체',
    step4Desc: '단계별 트래픽 전환',
    step5Title: '완료 및 정리',
    step5Desc: '전체 완료 및 자원 회수',
    currentActionDetails: '현재 상태',

    // NodeTopologyView
    nodeTopologyTitle: '노드 현황',
    nodeFilterAll: '전체',
    nodeFilterCanary: '카나리',
    nodeFilterStable: '스테이블',
    nodeFilterDraining: '드레인 중',
    activeSessions: '활성 세션 수',
    drainProgress: '드레인 진행률',
    versionBadge: '버전',

    // MetricCharts
    metricAnalysisTitle: '메트릭 및 임계치',
    metric5xx: '5xx 에러율',
    metricConnError: '커넥션 오류',
    metricLatency: 'P99 레이턴시',
    threshold: '임계치',
    safeZone: '정상',
    breached: '초과',
    healthyMetric: '정상',

    // UpgradeModal
    modalTitle: 'Envoy Gateway 업그레이드',
    modalSubtitle: '업그레이드할 목표 버전을 선택하세요',
    selectVersionLabel: '목표 릴리즈 버전',
    currentVersionLabel: '현재 버전',
    fetchingReleases: '릴리즈 목록 조회 중...',
    selectVersionPlaceholder: '버전 선택...',
    helmChartPreview: 'Helm OCI 차트:',
    btnCancel: '취소',
    btnLaunch: '업그레이드 시작:',
    btnLaunching: '시작 중...',
    prereleaseBadge: '프리릴리즈',
    latestBadge: '최신',

    // Footer
    footerText: 'Envoy Gateway Rollout Controller',
    noRollouts: '활성 롤아웃 리소스가 없습니다.',
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
