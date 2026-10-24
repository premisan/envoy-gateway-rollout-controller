package controller

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	appsv1 "k8s.io/api/apps/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rolloutv1alpha1 "github.com/envoy-gateway-rollout-controller/api/v1alpha1"
	"github.com/envoy-gateway-rollout-controller/pkg/analyzer"
	"github.com/envoy-gateway-rollout-controller/pkg/envoy"
	"github.com/envoy-gateway-rollout-controller/pkg/helm"
)

// EnvoyGatewayRolloutReconciler reconciles an EnvoyGatewayRollout object.
type EnvoyGatewayRolloutReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	AdminClient      *envoy.AdminClient
	PromManager      *analyzer.EphemeralPrometheusManager
	MetricAnalyzer   *analyzer.MetricAnalyzer
	StepStartTimes   map[string]time.Time
}

// NewEnvoyGatewayRolloutReconciler creates a new reconciler.
func NewEnvoyGatewayRolloutReconciler(
	c client.Client,
	scheme *runtime.Scheme,
) *EnvoyGatewayRolloutReconciler {
	return &EnvoyGatewayRolloutReconciler{
		Client:         c,
		Scheme:         scheme,
		AdminClient:    envoy.NewAdminClient(),
		PromManager:    analyzer.NewEphemeralPrometheusManager(c),
		MetricAnalyzer: analyzer.NewMetricAnalyzer(),
		StepStartTimes: make(map[string]time.Time),
	}
}

// Reconcile handles the core rollout state machine and sub-resource ownership for ArgoCD.
func (r *EnvoyGatewayRolloutReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	rollout := &rolloutv1alpha1.EnvoyGatewayRollout{}
	if err := r.Get(ctx, req.NamespacedName, rollout); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	targetNamespace := rollout.Spec.TargetNamespace
	if targetNamespace == "" {
		targetNamespace = "envoy-gateway-system"
	}

	// 1. Ensure target namespace exists
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: targetNamespace}}
	if err := r.Get(ctx, client.ObjectKey{Name: targetNamespace}, ns); errors.IsNotFound(err) {
		logger.Info("Creating target namespace", "namespace", targetNamespace)
		if err := r.Create(ctx, ns); err != nil && !errors.IsAlreadyExists(err) {
			return ctrl.Result{}, err
		}
	}

	// 2. Reconcile owned child resources (GatewayClass, EnvoyProxy, Gateway)
	// Setting OwnerReferences allows ArgoCD to track them cleanly in its application inventory
	if err := r.reconcileChildResources(ctx, rollout, targetNamespace); err != nil {
		logger.Error(err, "Failed to reconcile child resources")
		rollout.Status.Message = fmt.Sprintf("Child resource error: %v", err)
		_ = r.Status().Update(ctx, rollout)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Ensure base DaemonSet uses OnDelete strategy so Kubernetes never kills running proxy pods on spec update
	_ = r.ensureDaemonSetOnDelete(ctx, targetNamespace)

	// 3. Check for manual intervention (Pause / Abort)
	if rollout.Spec.Control.Abort {
		logger.Info("Manual abort requested via spec.control.abort")
		return r.handleAbort(ctx, rollout, "Manual rollback requested by operator")
	}

	if rollout.Spec.Control.Paused {
		if rollout.Status.Phase != rolloutv1alpha1.PhasePaused {
			logger.Info("Rollout paused by operator")
			rollout.Status.Phase = rolloutv1alpha1.PhasePaused
			rollout.Status.Message = "Rollout paused by operator"
			rollout.Status.LastUpdateTime = metav1.Now()
			_ = r.Status().Update(ctx, rollout)
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// 4. State Machine execution
	switch rollout.Status.Phase {
	case "", rolloutv1alpha1.PhasePending:
		return r.handlePending(ctx, rollout)
	case rolloutv1alpha1.PhaseDeploying, rolloutv1alpha1.PhaseCanaryRunning:
		return r.handleCanaryStep(ctx, rollout)
	case rolloutv1alpha1.PhaseObserving:
		return r.handleObserving(ctx, rollout)
	case rolloutv1alpha1.PhaseDraining:
		return r.handleDraining(ctx, rollout)
	case rolloutv1alpha1.PhaseRollingBack:
		return r.handleAbort(ctx, rollout, "Rolling back to stable state")
	case rolloutv1alpha1.PhaseCompleted:
		if rollout.Status.CurrentVersion != rollout.Spec.Version && !rollout.Spec.Control.Paused {
			return r.handlePending(ctx, rollout)
		}
		return r.handleCompleted(ctx, rollout)
	case rolloutv1alpha1.PhaseAborted:
		// If operator or GitOps sets a new target version while not in abort control, start new rollout!
		if rollout.Status.CurrentVersion != rollout.Spec.Version && !rollout.Spec.Control.Abort {
			logger.Info("New target version specified after abort, starting new rollout",
				"currentVersion", rollout.Status.CurrentVersion,
				"targetVersion", rollout.Spec.Version)
			return r.handlePending(ctx, rollout)
		}
		return ctrl.Result{}, nil
	case rolloutv1alpha1.PhasePaused:
		// If paused flag was cleared, resume rollout!
		if !rollout.Spec.Control.Paused {
			logger.Info("Rollout resumed by operator")
			if rollout.Status.CurrentStep > 0 {
				rollout.Status.Phase = rolloutv1alpha1.PhaseCanaryRunning
				rollout.Status.Message = fmt.Sprintf("Rollout resumed at Step %d", rollout.Status.CurrentStep)
			} else {
				rollout.Status.Phase = rolloutv1alpha1.PhasePending
				rollout.Status.Message = "Rollout resumed"
			}
			rollout.Status.LastUpdateTime = metav1.Now()
			_ = r.Status().Update(ctx, rollout)
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, nil
	default:
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
}

func (r *EnvoyGatewayRolloutReconciler) handlePending(ctx context.Context, rollout *rolloutv1alpha1.EnvoyGatewayRollout) (ctrl.Result, error) {
	rollout.Status.TargetVersion = rollout.Spec.Version
	rollout.Status.TotalSteps = len(rollout.Spec.Strategy.Steps)
	rollout.Status.StepTimeRemaining = ""

	if rollout.Status.CurrentVersion == "" {
		// Initial deployment scenario
		rollout.Status.CurrentVersion = rollout.Spec.Version
		rollout.Status.Phase = rolloutv1alpha1.PhaseCompleted
		rollout.Status.Message = fmt.Sprintf("Initial deployment of version %s completed", rollout.Spec.Version)
		rollout.Status.LastUpdateTime = metav1.Now()
		_ = r.Status().Update(ctx, rollout)
		return ctrl.Result{}, nil
	}

	if rollout.Status.CurrentVersion != rollout.Spec.Version {
		// New upgrade initiated
		rollout.Status.Phase = rolloutv1alpha1.PhaseDeploying
		rollout.Status.CurrentStep = 1
		rollout.Status.ConsecutiveFailures = 0
		rollout.Status.Message = fmt.Sprintf("Starting upgrade from %s to %s", rollout.Status.CurrentVersion, rollout.Spec.Version)
		rollout.Status.LastUpdateTime = metav1.Now()
		_ = r.Status().Update(ctx, rollout)
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{}, nil
}

// getActiveSessionsPerNode calculates real-time active downstream sessions per node from Prometheus or directly from Envoy pods.
func (r *EnvoyGatewayRolloutReconciler) getActiveSessionsPerNode(ctx context.Context, rollout *rolloutv1alpha1.EnvoyGatewayRollout, targetNamespace string) map[string]int {
	activeConnMap := make(map[string]int)
	promURL := rollout.Status.EphemeralPrometheusURL
	if promURL == "" {
		promURL = rollout.Spec.Analysis.PrometheusURL
	}

	allEnvoyPods := &corev1.PodList{}
	nodePodIPs := make(map[string][]string)
	allPodIPs := make([]string, 0)
	if err := r.List(ctx, allEnvoyPods, client.InNamespace(targetNamespace), client.MatchingLabels{"app.kubernetes.io/name": "envoy"}); err == nil {
		for _, pod := range allEnvoyPods.Items {
			if pod.Status.PodIP != "" && pod.Spec.NodeName != "" {
				nodePodIPs[pod.Spec.NodeName] = append(nodePodIPs[pod.Spec.NodeName], pod.Status.PodIP)
				allPodIPs = append(allPodIPs, pod.Status.PodIP)
			}
		}
	}

	if promURL != "" {
		if conns, err := r.MetricAnalyzer.QueryActiveConnections(ctx, promURL); err == nil && len(conns) > 0 {
			activeConnMap = conns
		}
	}

	// Fallback to direct pod scrape if Prometheus is unavailable or returned empty
	if len(activeConnMap) == 0 && len(allPodIPs) > 0 {
		activeConnMap = r.MetricAnalyzer.QueryPodActiveConnections(ctx, allPodIPs)
	}

	nodeSessions := make(map[string]int)
	for nodeName, ips := range nodePodIPs {
		total := 0
		for _, ip := range ips {
			total += activeConnMap[ip]
		}
		nodeSessions[nodeName] = total
	}
	return nodeSessions
}

func (r *EnvoyGatewayRolloutReconciler) handleCompleted(ctx context.Context, rollout *rolloutv1alpha1.EnvoyGatewayRollout) (ctrl.Result, error) {
	targetNamespace := rollout.Spec.TargetNamespace
	if targetNamespace == "" {
		targetNamespace = "envoy-gateway-system"
	}

	nodeSessions := r.getActiveSessionsPerNode(ctx, rollout, targetNamespace)

	changed := false
	if rollout.Status.StepTimeRemaining != "" {
		rollout.Status.StepTimeRemaining = ""
		changed = true
	}
	for i := range rollout.Status.Nodes {
		node := &rollout.Status.Nodes[i]
		curSessions := nodeSessions[node.NodeName]
		if node.ActiveSessions != curSessions {
			node.ActiveSessions = curSessions
			changed = true
		}
		if node.Phase != "Completed" {
			node.Phase = "Completed"
			changed = true
		}
		if node.DrainProgress != 100 {
			node.DrainProgress = 100
			changed = true
		}
	}

	if changed {
		rollout.Status.LastUpdateTime = metav1.Now()
		_ = r.Status().Update(ctx, rollout)
	}

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *EnvoyGatewayRolloutReconciler) handleCanaryStep(ctx context.Context, rollout *rolloutv1alpha1.EnvoyGatewayRollout) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch worker nodes matching NodeSelector
	nodeList := &corev1.NodeList{}
	listOpts := []client.ListOption{}
	if len(rollout.Spec.EnvoyProxy.NodeSelector) > 0 {
		listOpts = append(listOpts, client.MatchingLabels(rollout.Spec.EnvoyProxy.NodeSelector))
	}
	if err := r.List(ctx, nodeList, listOpts...); err != nil {
		return ctrl.Result{}, err
	}

	// Calculate target canary nodes for current step
	totalNodes := len(nodeList.Items)
	if totalNodes == 0 {
		totalNodes = 3 // Fallback default for demo/dev
	}

	if rollout.Status.TargetVersion == "" {
		rollout.Status.TargetVersion = rollout.Spec.Version
	}
	targetVersion := rollout.Status.TargetVersion

	targetNamespace := rollout.Spec.TargetNamespace
	if targetNamespace == "" {
		targetNamespace = "envoy-gateway-system"
	}

	// [Pre-Upgrade Metric Guardrail & Control Plane Sync]
	// Run only once upon initial transition into deploying phase to avoid 2-second repeat loops!
	if rollout.Status.Phase == rolloutv1alpha1.PhaseDeploying {
		if rollout.Spec.Analysis.EphemeralPrometheus.Enabled {
			ephemeralURL, err := r.PromManager.EnsureEphemeralPrometheus(ctx, rollout, nil)
			if err != nil {
				logger.Error(err, "Failed to spawn pre-upgrade ephemeral prometheus")
			} else {
				rollout.Status.EphemeralPrometheusURL = ephemeralURL
				logger.Info("Pre-upgrade ephemeral prometheus active prior to CRD synchronization", "url", ephemeralURL)
			}
		}

		// Synchronize CRDs and Envoy Gateway control plane deployment (keep data plane stable)
		_ = r.syncControlPlane(ctx, rollout, targetNamespace, targetVersion)
		rollout.Status.Phase = rolloutv1alpha1.PhaseCanaryRunning
	}

	stepIdx := rollout.Status.CurrentStep - 1
	if stepIdx < 0 || stepIdx >= len(rollout.Spec.Strategy.Steps) {
		rollout.Status.Phase = rolloutv1alpha1.PhaseCompleted
		rollout.Status.CurrentVersion = targetVersion
		rollout.Status.StepTimeRemaining = ""
		rollout.Status.Message = "Rollout successfully completed across all nodes"
		_ = r.syncDataPlaneDaemonSet(ctx, targetNamespace, targetVersion)
		r.deleteCanaryPods(ctx, targetNamespace)
		_ = r.PromManager.CleanupEphemeralPrometheus(ctx, rollout)
		_ = r.Status().Update(ctx, rollout)
		return ctrl.Result{}, nil
	}

	step := rollout.Spec.Strategy.Steps[stepIdx]
	targetCanaryCount := parseNodeCount(step.Nodes, totalNodes)

	nodeSessions := r.getActiveSessionsPerNode(ctx, rollout, targetNamespace)

	// Build node status list
	nodeStatuses := make([]rolloutv1alpha1.NodeRolloutStatus, 0)
	canaryNodeNames := make([]string, 0)

	for i := 0; i < totalNodes; i++ {
		nodeName := fmt.Sprintf("ingress-lb-node-%02d", i+1)
		nodeIP := fmt.Sprintf("10.244.1.%d", 10+i)
		if i < len(nodeList.Items) {
			nodeName = nodeList.Items[i].Name
			for _, addr := range nodeList.Items[i].Status.Addresses {
				if addr.Type == corev1.NodeInternalIP {
					nodeIP = addr.Address
					break
				}
			}
		}

		status := rolloutv1alpha1.NodeRolloutStatus{
			NodeName:       nodeName,
			NodeIP:         nodeIP,
			ActiveVersion:  rollout.Status.CurrentVersion,
			ActiveSessions: nodeSessions[nodeName],
		}

		if i < targetCanaryCount {
			status.Phase = "Canary"
			status.CanaryVersion = targetVersion
			canaryNodeNames = append(canaryNodeNames, nodeName)
		} else {
			status.Phase = "Stable"
		}
		nodeStatuses = append(nodeStatuses, status)
	}

	// [Overlapping Canary Pod Strategy - Plan A]
	// Spawn an overlapping canary pod directly on each canary node without touching the stable pod.
	// This ensures at least one Ready pod always exists on the node so CSP LoadBalancer with
	// externalTrafficPolicy: Local never encounters connection refused or RST packets.
	allCanaryPodsReady := true
	canaryPodIPs := make([]string, 0)
	readyCanaryPods := make([]*corev1.Pod, 0)
	unreadyNode := ""

	for _, cNode := range canaryNodeNames {
		pod, err := r.ensureCanaryPod(ctx, rollout, targetNamespace, cNode, targetVersion)
		if err != nil {
			logger.Error(err, "Failed to ensure overlapping canary pod", "node", cNode)
			allCanaryPodsReady = false
			unreadyNode = cNode
			continue
		}
		if isPodReady(pod) {
			canaryPodIPs = append(canaryPodIPs, pod.Status.PodIP)
			readyCanaryPods = append(readyCanaryPods, pod)
		} else {
			allCanaryPodsReady = false
			unreadyNode = cNode
		}
	}

	// If canary pods are still starting or pulling images, wait for them to become Ready
	if !allCanaryPodsReady {
		rollout.Status.Nodes = nodeStatuses
		rollout.Status.Phase = rolloutv1alpha1.PhaseCanaryRunning
		rollout.Status.Message = fmt.Sprintf("Starting overlapping canary pod on node %s (waiting for Ready)", unreadyNode)
		rollout.Status.LastUpdateTime = metav1.Now()
		_ = r.Status().Update(ctx, rollout)
		return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
	}

	// Register the Ready overlapping canary pods with custom EndpointSlice for the envoy-proxy Service!
	// This ensures zero packet loss and seamless load distribution without changing DaemonSet selector.
	if err := r.ensureCanaryEndpointSlice(ctx, targetNamespace, readyCanaryPods); err != nil {
		logger.Error(err, "Failed to update canary EndpointSlice")
	}

	// Ensure Ephemeral Prometheus monitors the overlapping canary pods
	if rollout.Spec.Analysis.EphemeralPrometheus.Enabled && len(canaryPodIPs) > 0 {
		ephemeralURL, err := r.PromManager.EnsureEphemeralPrometheus(ctx, rollout, canaryPodIPs)
		if err != nil {
			logger.Error(err, "Failed to spawn ephemeral prometheus")
		} else {
			rollout.Status.EphemeralPrometheusURL = ephemeralURL
		}
	}

	rollout.Status.Nodes = nodeStatuses
	rollout.Status.Phase = rolloutv1alpha1.PhaseObserving
	rollout.Status.Message = fmt.Sprintf("Overlapping canary pod Ready on %d node(s). Observing Step %d/%d (%s)", len(canaryNodeNames), rollout.Status.CurrentStep, rollout.Status.TotalSteps, step.Nodes)
	rollout.Status.LastUpdateTime = metav1.Now()

	// Store step start time
	key := fmt.Sprintf("%s/%d", rollout.Name, rollout.Status.CurrentStep)
	r.StepStartTimes[key] = time.Now()

	nodeSessions = r.getActiveSessionsPerNode(ctx, rollout, targetNamespace)
	for i := range rollout.Status.Nodes {
		rollout.Status.Nodes[i].ActiveSessions = nodeSessions[rollout.Status.Nodes[i].NodeName]
	}

	_ = r.Status().Update(ctx, rollout)
	return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
}

func (r *EnvoyGatewayRolloutReconciler) handleObserving(ctx context.Context, rollout *rolloutv1alpha1.EnvoyGatewayRollout) (ctrl.Result, error) {
	targetNamespace := rollout.Spec.TargetNamespace
	if targetNamespace == "" {
		targetNamespace = "envoy-gateway-system"
	}

	stepIdx := rollout.Status.CurrentStep - 1
	step := rollout.Spec.Strategy.Steps[stepIdx]

	key := fmt.Sprintf("%s/%d", rollout.Name, rollout.Status.CurrentStep)
	startTime, exists := r.StepStartTimes[key]
	if !exists {
		startTime = time.Now()
		r.StepStartTimes[key] = startTime
	}

	pauseDuration, _ := time.ParseDuration(step.PauseDuration)
	if pauseDuration == 0 {
		pauseDuration = 3 * time.Minute
	}

	elapsed := time.Since(startTime)
	remaining := pauseDuration - elapsed
	if remaining < 0 {
		remaining = 0
	}
	rollout.Status.StepTimeRemaining = remaining.Round(time.Second).String()

	// Metric Evaluation
	promURL := rollout.Status.EphemeralPrometheusURL
	if promURL == "" {
		promURL = rollout.Spec.Analysis.PrometheusURL
	}

	canaryNodes := make([]string, 0)
	for _, n := range rollout.Status.Nodes {
		if n.Phase == "Canary" {
			canaryNodes = append(canaryNodes, n.NodeName)
			if n.NodeIP != "" {
				canaryNodes = append(canaryNodes, n.NodeIP)
			}
		}
	}

	// Also append envoy-proxy pod IPs running on canary nodes
	activeEnvoyPods := &corev1.PodList{}
	if err := r.List(ctx, activeEnvoyPods, client.InNamespace(rollout.Spec.TargetNamespace), client.MatchingLabels{"app.kubernetes.io/name": "envoy"}); err == nil {
		for _, pod := range activeEnvoyPods.Items {
			if pod.Status.PodIP != "" {
				for _, cn := range canaryNodes {
					if pod.Spec.NodeName == cn {
						canaryNodes = append(canaryNodes, pod.Status.PodIP)
						break
					}
				}
			}
		}
	}

	rules := rollout.Spec.Analysis.Metrics
	if len(rules) == 0 {
		// Default rules if unspecified
		rules = []rolloutv1alpha1.MetricRule{
			{
				Name:      "http-5xx-rate",
				Threshold: 1.0,
				Query:     `sum(rate(envoy_cluster_upstream_rq_xx{response_code_class="5"}[1m])) / clamp_min(sum(rate(envoy_cluster_upstream_rq_xx[1m])), 0.001) * 100`,
			},
			{
				Name:      "http-4xx-rate",
				Threshold: 5.0,
				Query:     `(sum(rate(envoy_http_downstream_rq_xx{envoy_response_code_class="4"}[1m])) or sum(rate(envoy_cluster_upstream_rq_xx{envoy_response_code_class="4"}[1m])) or sum(rate(envoy_cluster_upstream_rq_xx{response_code_class="4"}[1m]))) / clamp_min(sum(rate(envoy_http_downstream_rq_xx[1m])) or sum(rate(envoy_cluster_upstream_rq_xx[1m])), 0.001) * 100`,
			},
		}
	}

	analysisResult, err := r.MetricAnalyzer.Evaluate(
		ctx,
		promURL,
		rules,
		canaryNodes,
		rollout.Status.ConsecutiveFailures,
		rollout.Spec.Analysis.FailureLimit,
	)

	if err == nil {
		rollout.Status.ConsecutiveFailures = analysisResult.ConsecutiveFailures
		rollout.Status.LastMetricSnapshot = analysisResult.MetricsSnapshot

		// Auto-rollback condition
		if !analysisResult.AllHealthy {
			msg := fmt.Sprintf("Metric anomalies detected exceeding limit: %s", strings.Join(analysisResult.ViolationMessages, ", "))
			return r.handleAbort(ctx, rollout, msg)
		}
	}

	// Check if step observation time passed or manual ForcePromote requested
	if remaining == 0 || rollout.Spec.Control.ForcePromote {
		if rollout.Spec.Control.ForcePromote {
			rollout.Spec.Control.ForcePromote = false
			_ = r.Update(ctx, rollout)
		}
		rollout.Status.Phase = rolloutv1alpha1.PhaseDraining
		rollout.Status.Message = fmt.Sprintf("Step %d observation passed. Draining old listeners on canary nodes.", rollout.Status.CurrentStep)
		_ = r.Status().Update(ctx, rollout)
		return ctrl.Result{Requeue: true}, nil
	}

	nodeSessions := r.getActiveSessionsPerNode(ctx, rollout, targetNamespace)
	for i := range rollout.Status.Nodes {
		rollout.Status.Nodes[i].ActiveSessions = nodeSessions[rollout.Status.Nodes[i].NodeName]
	}

	_ = r.Status().Update(ctx, rollout)
	return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
}

func (r *EnvoyGatewayRolloutReconciler) handleDraining(ctx context.Context, rollout *rolloutv1alpha1.EnvoyGatewayRollout) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	targetNamespace := rollout.Spec.TargetNamespace
	if targetNamespace == "" {
		targetNamespace = "envoy-gateway-system"
	}
	targetVersion := rollout.Status.TargetVersion
	if targetVersion == "" {
		targetVersion = rollout.Spec.Version
	}

	// 1. If concluding the final step, ensure base DaemonSet template is synced to targetVersion first,
	// so that when old pods are deleted, Kubernetes automatically schedules the new version pods!
	if rollout.Status.CurrentStep >= rollout.Status.TotalSteps {
		_ = r.syncDataPlaneDaemonSet(ctx, targetNamespace, targetVersion)
	}

	// Trigger Envoy Admin API listener drain on OLD stable instances on canary nodes.
	// Since the overlapping canary pod is already Ready and handling traffic on the node,
	// new connections seamlessly shift to the canary pod without dropping a single packet.
	podList := &corev1.PodList{}
	_ = r.List(ctx, podList, client.InNamespace(targetNamespace), client.MatchingLabels{"app.kubernetes.io/name": "envoy"})

	for i := range rollout.Status.Nodes {
		node := &rollout.Status.Nodes[i]
		if node.Phase == "Canary" {
			node.Phase = "Draining"
			node.DrainProgress = 50

			for _, pod := range podList.Items {
				// Target older stable pod (not the overlapping canary pod)
				if pod.Spec.NodeName == node.NodeName && pod.Labels["rollout.envoyproxy.io/canary"] != "true" && pod.DeletionTimestamp == nil {
					logger.Info("Draining older stable envoy-proxy pod on canary node", "node", node.NodeName, "pod", pod.Name, "ip", pod.Status.PodIP)
					if pod.Status.PodIP != "" {
						_ = r.AdminClient.DrainListeners(ctx, pod.Status.PodIP, 19001)
					}
					// Gracefully delete the older stable pod; the overlapping canary pod is already serving!
					_ = r.Delete(ctx, &pod)
				}
			}
		}
	}

	logger.Info("Completed draining old listeners on nodes")
	// Advance to next step
	if rollout.Status.CurrentStep < rollout.Status.TotalSteps {
		rollout.Status.CurrentStep++
		rollout.Status.Phase = rolloutv1alpha1.PhaseCanaryRunning
		rollout.Status.Message = fmt.Sprintf("Advancing to Step %d/%d", rollout.Status.CurrentStep, rollout.Status.TotalSteps)
	} else {
		// All steps reaching conclusion: verify updated base DaemonSet pods are 100% Ready before terminating canary!
		completedVer := targetVersion
		_ = r.syncDataPlaneDaemonSet(ctx, targetNamespace, completedVer)

		freshPodList := &corev1.PodList{}
		_ = r.List(ctx, freshPodList, client.InNamespace(targetNamespace), client.MatchingLabels{"app.kubernetes.io/name": "envoy"})

		allNodesHaveReadyUpdatedPod := true
		var unreadyNodeName string
		for _, node := range rollout.Status.Nodes {
			hasReadyUpdated := false
			for _, pod := range freshPodList.Items {
				if pod.Spec.NodeName == node.NodeName && pod.Labels["rollout.envoyproxy.io/canary"] != "true" && isPodReady(&pod) && pod.DeletionTimestamp == nil {
					hasReadyUpdated = true
					break
				}
			}
			if !hasReadyUpdated {
				allNodesHaveReadyUpdatedPod = false
				unreadyNodeName = node.NodeName
				break
			}
		}

		// If new base DaemonSet pod is still starting or pulling image, DO NOT terminate canary!
		if !allNodesHaveReadyUpdatedPod {
			logger.Info("Waiting for updated envoy-proxy pod to become Ready before completing rollout", "unreadyNode", unreadyNodeName)
			rollout.Status.Phase = rolloutv1alpha1.PhaseDraining
			rollout.Status.Message = fmt.Sprintf("Waiting for updated envoy-proxy pod on node %s to become Ready before completing rollout", unreadyNodeName)
			rollout.Status.LastUpdateTime = metav1.Now()
			_ = r.Status().Update(ctx, rollout)
			return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
		}

		// All updated base pods are confirmed Ready! Safe to complete and remove canary pods.
		rollout.Status.Phase = rolloutv1alpha1.PhaseCompleted
		rollout.Status.CurrentVersion = completedVer
		rollout.Status.StepTimeRemaining = ""
		rollout.Status.Message = fmt.Sprintf("All %d steps successfully completed. Envoy Gateway upgraded to %s", rollout.Status.TotalSteps, completedVer)
		for i := range rollout.Status.Nodes {
			rollout.Status.Nodes[i].Phase = "Completed"
			rollout.Status.Nodes[i].ActiveVersion = completedVer
			rollout.Status.Nodes[i].CanaryVersion = ""
			rollout.Status.Nodes[i].DrainProgress = 100
		}
		// Clean up temporary overlapping canary pods (only after updated base pods are confirmed Ready!)
		r.deleteCanaryPods(ctx, targetNamespace)
		// Clean up temporary Prometheus
		_ = r.PromManager.CleanupEphemeralPrometheus(ctx, rollout)
	}

	rollout.Status.LastUpdateTime = metav1.Now()
	nodeSessions := r.getActiveSessionsPerNode(ctx, rollout, targetNamespace)
	for i := range rollout.Status.Nodes {
		rollout.Status.Nodes[i].ActiveSessions = nodeSessions[rollout.Status.Nodes[i].NodeName]
	}

	_ = r.Status().Update(ctx, rollout)
	return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
}

func (r *EnvoyGatewayRolloutReconciler) handleAbort(ctx context.Context, rollout *rolloutv1alpha1.EnvoyGatewayRollout, reason string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Executing zero-downtime rollout abort and restoring stable state", "reason", reason)

	stableVersion := rollout.Status.CurrentVersion
	if stableVersion == "" {
		stableVersion = rollout.Spec.Version
	}

	targetNamespace := rollout.Spec.TargetNamespace
	if targetNamespace == "" {
		targetNamespace = "envoy-gateway-system"
	}

	// 1. Clear control flags and align spec.version to stableVersion to prevent drift retry loops
	rollout.Spec.Control.Abort = false
	rollout.Spec.Control.Paused = false
	rollout.Spec.Control.ForcePromote = false
	rollout.Spec.Version = stableVersion
	if err := r.Update(ctx, rollout); err != nil {
		logger.Error(err, "Failed to update spec during abort")
	}

	// 2. Ensure base DaemonSet uses OnDelete strategy so existing stable pods are NEVER killed
	_ = r.ensureDaemonSetOnDelete(ctx, targetNamespace)

	// 3. Inspect running pods on all target nodes
	podList := &corev1.PodList{}
	_ = r.List(ctx, podList, client.InNamespace(targetNamespace), client.MatchingLabels{"app.kubernetes.io/name": "envoy"})

	allNodesHaveReadyStable := true
	var unreadyNode string
	for _, node := range rollout.Status.Nodes {
		hasReadyStable := false
		for _, pod := range podList.Items {
			// Find stable pod (not temporary canary) on this node that is fully Ready
			if pod.Spec.NodeName == node.NodeName && pod.Labels["rollout.envoyproxy.io/canary"] != "true" && isPodReady(&pod) {
				hasReadyStable = true
				break
			}
		}
		if !hasReadyStable {
			allNodesHaveReadyStable = false
			unreadyNode = node.NodeName
			break
		}
	}

	// If any node lacks a Ready stable pod (e.g. if rollback occurred during/after drain),
	// DO NOT delete canary pods yet! Keep them serving traffic via EndpointSlice until stable pod recovers.
	if !allNodesHaveReadyStable {
		logger.Info("Rollback in progress: waiting for stable daemonset pod to become Ready before terminating canary", "unreadyNode", unreadyNode)
		rollout.Status.Phase = rolloutv1alpha1.PhaseRollingBack
		rollout.Status.Message = fmt.Sprintf("Rollback in progress: waiting for stable pod on node %s to become Ready", unreadyNode)
		rollout.Status.LastUpdateTime = metav1.Now()
		_ = r.Status().Update(ctx, rollout)
		return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
	}

	// 4. All nodes now have a Ready stable pod serving traffic!
	// Remove custom EndpointSlice first so all incoming traffic shifts back to stable pods.
	_ = r.ensureCanaryEndpointSlice(ctx, targetNamespace, nil)

	// Gracefully drain and delete overlapping canary pods
	canaryPodList := &corev1.PodList{}
	if err := r.List(ctx, canaryPodList, client.InNamespace(targetNamespace), client.MatchingLabels{"rollout.envoyproxy.io/canary": "true"}); err == nil {
		for _, pod := range canaryPodList.Items {
			logger.Info("Gracefully draining and removing overlapping canary pod during rollback", "pod", pod.Name, "node", pod.Spec.NodeName)
			if pod.Status.PodIP != "" {
				_ = r.AdminClient.DrainListeners(ctx, pod.Status.PodIP, 19001)
			}
			_ = r.Delete(ctx, &pod)
		}
	}

	// 5. Restore control plane components to stableVersion (without touching data plane daemonset!)
	_ = r.syncControlPlane(ctx, rollout, targetNamespace, stableVersion)

	// 6. Update status to Aborted terminal state
	rollout.Status.Phase = rolloutv1alpha1.PhaseAborted
	rollout.Status.TargetVersion = stableVersion
	rollout.Status.Message = fmt.Sprintf("Rollout safely aborted with zero downtime and restored to %s: %s", stableVersion, reason)
	rollout.Status.StepTimeRemaining = ""

	for i := range rollout.Status.Nodes {
		rollout.Status.Nodes[i].Phase = "RolledBack"
		rollout.Status.Nodes[i].ActiveVersion = stableVersion
		rollout.Status.Nodes[i].CanaryVersion = ""
		rollout.Status.Nodes[i].DrainProgress = 0
	}

	_ = r.PromManager.CleanupEphemeralPrometheus(ctx, rollout)
	rollout.Status.LastUpdateTime = metav1.Now()
	_ = r.Status().Update(ctx, rollout)
	return ctrl.Result{}, nil
}

// reconcileChildResources creates or updates GatewayClass, EnvoyProxy, and Gateway
// with OwnerReferences pointing to EnvoyGatewayRollout for clean ArgoCD topology representation.
func (r *EnvoyGatewayRolloutReconciler) reconcileChildResources(
	ctx context.Context,
	rollout *rolloutv1alpha1.EnvoyGatewayRollout,
	targetNamespace string,
) error {
	ownerRef := metav1.OwnerReference{
		APIVersion: "rollout.envoyproxy.io/v1alpha1",
		Kind:       "EnvoyGatewayRollout",
		Name:       rollout.Name,
		UID:        rollout.UID,
	}

	// 1. GatewayClass
	gwClassName := rollout.Spec.Gateway.GatewayClassName
	if gwClassName == "" {
		gwClassName = "eg"
	}
	gwClass := &unstructured.Unstructured{}
	gwClass.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "gateway.networking.k8s.io",
		Version: "v1",
		Kind:    "GatewayClass",
	})
	gwClass.SetName(gwClassName)
	gwClass.SetOwnerReferences([]metav1.OwnerReference{ownerRef})
	_ = unstructured.SetNestedField(gwClass.Object, "gateway.envoyproxy.io/gatewayclass-controller", "spec", "controllerName")
	_ = r.Create(ctx, gwClass) // Ignore if already exists

	// 2. EnvoyProxy (matches user's real configuration with nodeSelector and tolerations)
	envoyProxyName := rollout.Spec.EnvoyProxy.Name
	if envoyProxyName == "" {
		envoyProxyName = "proxy-config"
	}
	envoyProxy := &unstructured.Unstructured{}
	envoyProxy.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "gateway.envoyproxy.io",
		Version: "v1alpha1",
		Kind:    "EnvoyProxy",
	})
	envoyProxy.SetName(envoyProxyName)
	envoyProxy.SetNamespace(targetNamespace)
	envoyProxy.SetOwnerReferences([]metav1.OwnerReference{ownerRef})

	daemonSetSpec := map[string]interface{}{
		"name": "envoy-proxy",
		"strategy": map[string]interface{}{
			"type": "OnDelete",
		},
	}
	if len(rollout.Spec.EnvoyProxy.NodeSelector) > 0 {
		daemonSetSpec["pod"] = map[string]interface{}{
			"nodeSelector": stringMapToInterfaceMap(rollout.Spec.EnvoyProxy.NodeSelector),
		}
	}
	_ = unstructured.SetNestedMap(envoyProxy.Object, map[string]interface{}{
		"type": "Kubernetes",
		"kubernetes": map[string]interface{}{
			"envoyDaemonSet": daemonSetSpec,
			"envoyService": map[string]interface{}{
				"name":        "envoy-proxy",
				"annotations": stringMapToInterfaceMap(rollout.Spec.EnvoyProxy.Annotations),
			},
		},
	}, "spec", "provider")

	existingProxy := &unstructured.Unstructured{}
	existingProxy.SetGroupVersionKind(envoyProxy.GroupVersionKind())
	if err := r.Get(ctx, client.ObjectKey{Namespace: targetNamespace, Name: envoyProxyName}, existingProxy); errors.IsNotFound(err) {
		_ = r.Create(ctx, envoyProxy)
	} else if err == nil {
		_ = unstructured.SetNestedMap(existingProxy.Object, map[string]interface{}{
			"type": "Kubernetes",
			"kubernetes": map[string]interface{}{
				"envoyDaemonSet": daemonSetSpec,
				"envoyService": map[string]interface{}{
					"name":        "envoy-proxy",
					"annotations": stringMapToInterfaceMap(rollout.Spec.EnvoyProxy.Annotations),
				},
			},
		}, "spec", "provider")
		_ = r.Update(ctx, existingProxy)
	}

	// 3. Gateway Resource
	gwName := rollout.Spec.Gateway.Name
	if gwName == "" {
		gwName = "eg"
	}
	gw := &unstructured.Unstructured{}
	gw.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "gateway.networking.k8s.io",
		Version: "v1",
		Kind:    "Gateway",
	})
	gw.SetName(gwName)
	gw.SetNamespace(targetNamespace)
	gw.SetOwnerReferences([]metav1.OwnerReference{ownerRef})

	listeners := make([]interface{}, 0)
	for _, l := range rollout.Spec.Gateway.Listeners {
		listenerObj := map[string]interface{}{
			"name":     l.Name,
			"protocol": l.Protocol,
			"port":     int64(l.Port),
			"allowedRoutes": map[string]interface{}{
				"namespaces": map[string]interface{}{
					"from": "All",
				},
			},
		}
		if l.TLSSecretName != "" {
			listenerObj["tls"] = map[string]interface{}{
				"mode": "Terminate",
				"certificateRefs": []interface{}{
					map[string]interface{}{
						"kind":  "Secret",
						"group": "",
						"name":  l.TLSSecretName,
					},
				},
			}
		}
		listeners = append(listeners, listenerObj)
	}

	_ = unstructured.SetNestedField(gw.Object, gwClassName, "spec", "gatewayClassName")
	_ = unstructured.SetNestedMap(gw.Object, map[string]interface{}{
		"parametersRef": map[string]interface{}{
			"group": "gateway.envoyproxy.io",
			"kind":  "EnvoyProxy",
			"name":  envoyProxyName,
		},
	}, "spec", "infrastructure")
	_ = unstructured.SetNestedSlice(gw.Object, listeners, "spec", "listeners")
	_ = r.Create(ctx, gw)

	// 4. DataPlane DaemonSet: link OwnerReference to EnvoyGatewayRollout for ArgoCD resource tree visibility
	dsList := &appsv1.DaemonSetList{}
	if err := r.List(ctx, dsList, client.InNamespace(targetNamespace)); err == nil {
		for i := range dsList.Items {
			ds := &dsList.Items[i]
			if ds.Labels["gateway.envoyproxy.io/owning-gateway-name"] == gwName || ds.Name == "envoy-proxy" {
				newRefs, changed := appendOwnerReference(ds.OwnerReferences, ownerRef)
				if changed {
					ds.OwnerReferences = newRefs
					_ = r.Update(ctx, ds)
				}
			}
		}
	}

	// 5. DataPlane Service: link OwnerReference to EnvoyGatewayRollout for ArgoCD resource tree visibility
	svcList := &corev1.ServiceList{}
	if err := r.List(ctx, svcList, client.InNamespace(targetNamespace)); err == nil {
		for i := range svcList.Items {
			svc := &svcList.Items[i]
			if svc.Labels["gateway.envoyproxy.io/owning-gateway-name"] == gwName || svc.Name == "envoy-proxy" {
				newRefs, changed := appendOwnerReference(svc.OwnerReferences, ownerRef)
				if changed {
					svc.OwnerReferences = newRefs
					_ = r.Update(ctx, svc)
				}
			}
		}
	}

	return nil
}

func appendOwnerReference(existing []metav1.OwnerReference, target metav1.OwnerReference) ([]metav1.OwnerReference, bool) {
	for i, ref := range existing {
		if ref.Kind == target.Kind && ref.Name == target.Name {
			if ref.UID != target.UID || ref.APIVersion != target.APIVersion {
				existing[i].UID = target.UID
				existing[i].APIVersion = target.APIVersion
				return existing, true
			}
			return existing, false
		}
	}
	return append(existing, target), true
}

func parseNodeCount(nodesStr string, totalNodes int) int {
	nodesStr = strings.TrimSpace(nodesStr)
	if strings.HasSuffix(nodesStr, "%") {
		pctStr := strings.TrimSuffix(nodesStr, "%")
		pct, err := strconv.Atoi(pctStr)
		if err == nil {
			cnt := (totalNodes * pct) / 100
			if cnt <= 0 {
				return 1
			}
			return cnt
		}
	}
	cnt, err := strconv.Atoi(nodesStr)
	if err == nil {
		if cnt > totalNodes {
			return totalNodes
		}
		return cnt
	}
	return 1
}

func stringMapToInterfaceMap(m map[string]string) map[string]interface{} {
	res := make(map[string]interface{}, len(m))
	for k, v := range m {
		res[k] = v
	}
	return res
}

// syncControlPlane synchronizes CRDs and updates the Envoy Gateway Control Plane Deployment.
func (r *EnvoyGatewayRolloutReconciler) syncControlPlane(
	ctx context.Context,
	rollout *rolloutv1alpha1.EnvoyGatewayRollout,
	targetNamespace string,
	targetVersion string,
) error {
	logger := log.FromContext(ctx)

	// 0. Synchronize CRDs and Envoy Gateway stack using official Helm OCI chart
	releaseName := rollout.Spec.Gateway.Name
	if releaseName == "" {
		releaseName = "eg"
	}
	if err := helm.SyncHelmRelease(ctx, releaseName, targetNamespace, targetVersion); err != nil {
		logger.Info("Helm OCI sync notice (continuing with direct resource reconciliation)", "detail", err.Error())
	}

	// 1. Update Envoy Gateway Control Plane Deployment
	deploy := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Namespace: targetNamespace, Name: "envoy-gateway"}, deploy)
	if err == nil {
		updated := false
		targetGatewayImage := fmt.Sprintf("envoyproxy/gateway:%s", targetVersion)
		for i := range deploy.Spec.Template.Spec.Containers {
			c := &deploy.Spec.Template.Spec.Containers[i]
			if c.Name == "envoy-gateway" && !imageEquals(c.Image, targetGatewayImage) {
				logger.Info("Updating envoy-gateway deployment image", "from", c.Image, "to", targetGatewayImage)
				c.Image = targetGatewayImage
				updated = true
			}
		}
		if updated {
			if err := r.Update(ctx, deploy); err != nil {
				logger.Error(err, "Failed to update envoy-gateway deployment image")
			}
		}
	}
	return nil
}

// syncDataPlaneDaemonSet updates the base envoy-proxy DaemonSet to targetVersion upon full rollout completion.
func (r *EnvoyGatewayRolloutReconciler) syncDataPlaneDaemonSet(
	ctx context.Context,
	targetNamespace string,
	targetVersion string,
) error {
	logger := log.FromContext(ctx)

	ds := &appsv1.DaemonSet{}
	err := r.Get(ctx, client.ObjectKey{Namespace: targetNamespace, Name: "envoy-proxy"}, ds)
	if err == nil {
		updated := false
		targetGatewayImage := fmt.Sprintf("envoyproxy/gateway:%s", targetVersion)
		targetEnvoyImage := resolveEnvoyImageForVersion(targetVersion)

		for i := range ds.Spec.Template.Spec.Containers {
			c := &ds.Spec.Template.Spec.Containers[i]
			if c.Name == "shutdown-manager" && !imageEquals(c.Image, targetGatewayImage) {
				logger.Info("Updating envoy-proxy daemonset shutdown-manager image", "from", c.Image, "to", targetGatewayImage)
				c.Image = targetGatewayImage
				updated = true
			}
			if c.Name == "envoy" && !imageEquals(c.Image, targetEnvoyImage) {
				logger.Info("Updating envoy-proxy daemonset envoy image", "from", c.Image, "to", targetEnvoyImage)
				c.Image = targetEnvoyImage
				updated = true
			}
		}
		if updated {
			if err := r.Update(ctx, ds); err != nil {
				logger.Error(err, "Failed to update envoy-proxy daemonset images")
			}
		}
	}
	return nil
}

// ensureCanaryPod creates or retrieves an overlapping canary pod on the specified node.
// This pod runs with targetVersion images while the stable daemonset pod continues serving on the same node.
func (r *EnvoyGatewayRolloutReconciler) ensureCanaryPod(
	ctx context.Context,
	rollout *rolloutv1alpha1.EnvoyGatewayRollout,
	targetNamespace string,
	nodeName string,
	targetVersion string,
) (*corev1.Pod, error) {
	podName := fmt.Sprintf("envoy-proxy-canary-%s", sanitizeResourceName(nodeName))

	existingPod := &corev1.Pod{}
	err := r.Get(ctx, client.ObjectKey{Namespace: targetNamespace, Name: podName}, existingPod)
	if err == nil {
		return existingPod, nil
	}
	if !errors.IsNotFound(err) {
		return nil, err
	}

	// Fetch base DaemonSet template
	ds := &appsv1.DaemonSet{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: targetNamespace, Name: "envoy-proxy"}, ds); err != nil {
		return nil, fmt.Errorf("failed to get base envoy-proxy daemonset: %w", err)
	}

	targetGatewayImage := fmt.Sprintf("envoyproxy/gateway:%s", targetVersion)
	targetEnvoyImage := resolveEnvoyImageForVersion(targetVersion)

	podSpec := *ds.Spec.Template.Spec.DeepCopy()
	podSpec.NodeName = nodeName // Explicitly schedule on target canary node

	for i := range podSpec.Containers {
		c := &podSpec.Containers[i]
		if c.Name == "envoy" {
			c.Image = targetEnvoyImage
		}
		if c.Name == "shutdown-manager" {
			c.Image = targetGatewayImage
		}
	}

	ownerRef := metav1.OwnerReference{
		APIVersion: rollout.APIVersion,
		Kind:       rollout.Kind,
		Name:       rollout.Name,
		UID:        rollout.UID,
	}

	labels := make(map[string]string)
	for k, v := range ds.Spec.Template.Labels {
		labels[k] = v
	}
	// CRITICAL FIX: Do NOT match the base DaemonSet's exact selector!
	// Kubernetes daemonset-controller monitors spec.selector matches per node,
	// and if multiple pods on the same node match, daemonset-controller IMMEDIATELY
	// deletes the surplus pod (causing the Create -> Terminate loop).
	// By changing managed-by and component labels, daemonset-controller leaves our canary pod alone!
	labels["app.kubernetes.io/managed-by"] = "envoy-gateway-rollout-controller"
	labels["app.kubernetes.io/component"] = "canary-proxy"
	labels["rollout.envoyproxy.io/canary"] = "true"
	labels["rollout.envoyproxy.io/node"] = nodeName

	canaryPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            podName,
			Namespace:       targetNamespace,
			Labels:          labels,
			Annotations:     ds.Spec.Template.Annotations,
			OwnerReferences: []metav1.OwnerReference{ownerRef},
		},
		Spec: podSpec,
	}

	if err := r.Create(ctx, canaryPod); err != nil {
		return nil, fmt.Errorf("failed to create overlapping canary pod on node %s: %w", nodeName, err)
	}

	return canaryPod, nil
}

// ensureCanaryEndpointSlice creates or updates a custom EndpointSlice attached to the envoy-proxy Service.
// Kube-proxy aggregates all EndpointSlices for the service, routing NodePort traffic to the overlapping canary pod.
func (r *EnvoyGatewayRolloutReconciler) ensureCanaryEndpointSlice(
	ctx context.Context,
	targetNamespace string,
	canaryPods []*corev1.Pod,
) error {
	sliceName := "envoy-proxy-canary"

	var endpoints []discoveryv1.Endpoint
	for _, pod := range canaryPods {
		if pod != nil && pod.Status.PodIP != "" && isPodReady(pod) {
			ready := true
			serving := true
			terminating := false
			nodeName := pod.Spec.NodeName
			endpoints = append(endpoints, discoveryv1.Endpoint{
				Addresses: []string{pod.Status.PodIP},
				Conditions: discoveryv1.EndpointConditions{
					Ready:       &ready,
					Serving:     &serving,
					Terminating: &terminating,
				},
				NodeName: &nodeName,
				TargetRef: &corev1.ObjectReference{
					Kind:      "Pod",
					Name:      pod.Name,
					Namespace: pod.Namespace,
					UID:       pod.UID,
				},
			})
		}
	}

	if len(endpoints) == 0 {
		slice := &discoveryv1.EndpointSlice{}
		err := r.Get(ctx, client.ObjectKey{Namespace: targetNamespace, Name: sliceName}, slice)
		if err == nil {
			_ = r.Delete(ctx, slice)
		}
		return nil
	}

	var slicePorts []discoveryv1.EndpointPort
	svc := &corev1.Service{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: targetNamespace, Name: "envoy-proxy"}, svc); err == nil && len(svc.Spec.Ports) > 0 {
		for _, sp := range svc.Spec.Ports {
			portNum := sp.TargetPort.IntVal
			if portNum == 0 {
				portNum = sp.Port
			}
			pName := sp.Name
			proto := sp.Protocol
			slicePorts = append(slicePorts, discoveryv1.EndpointPort{
				Name:     &pName,
				Port:     &portNum,
				Protocol: &proto,
			})
		}
	} else {
		tcpProto := corev1.ProtocolTCP
		portHttp := int32(10080)
		nameHttp := "http-80"
		portHttps := int32(10443)
		nameHttps := "https-443"
		slicePorts = []discoveryv1.EndpointPort{
			{Name: &nameHttp, Port: &portHttp, Protocol: &tcpProto},
			{Name: &nameHttps, Port: &portHttps, Protocol: &tcpProto},
		}
	}

	slice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sliceName,
			Namespace: targetNamespace,
			Labels: map[string]string{
				"kubernetes.io/service-name":             "envoy-proxy",
				"endpointslice.kubernetes.io/managed-by": "envoy-gateway-rollout-controller",
			},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints:   endpoints,
		Ports:       slicePorts,
	}

	existingSlice := &discoveryv1.EndpointSlice{}
	err := r.Get(ctx, client.ObjectKey{Namespace: targetNamespace, Name: sliceName}, existingSlice)
	if errors.IsNotFound(err) {
		return r.Create(ctx, slice)
	} else if err == nil {
		existingSlice.Endpoints = slice.Endpoints
		existingSlice.Ports = slice.Ports
		return r.Update(ctx, existingSlice)
	}
	return err
}

// deleteCanaryPods cleans up all overlapping canary pods and their EndpointSlice.
func (r *EnvoyGatewayRolloutReconciler) deleteCanaryPods(ctx context.Context, targetNamespace string) {
	logger := log.FromContext(ctx)
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace(targetNamespace), client.MatchingLabels{"rollout.envoyproxy.io/canary": "true"}); err == nil {
		for _, pod := range podList.Items {
			logger.Info("Cleaning up overlapping canary pod", "pod", pod.Name, "node", pod.Spec.NodeName)
			_ = r.Delete(ctx, &pod)
		}
	}
	_ = r.ensureCanaryEndpointSlice(ctx, targetNamespace, nil)
}

// isPodReady checks whether a pod has passed its readiness probes and is currently in Ready condition.
func isPodReady(pod *corev1.Pod) bool {
	if pod == nil || pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// sanitizeResourceName cleanses node names for use in Kubernetes pod names.
func sanitizeResourceName(name string) string {
	res := strings.ToLower(name)
	res = strings.ReplaceAll(res, ".", "-")
	res = strings.ReplaceAll(res, "_", "-")
	if len(res) > 35 {
		res = res[:35]
	}
	return strings.Trim(res, "-")
}

// imageEquals normalizes docker registry prefixes to avoid false positive diffs.
func imageEquals(img1, img2 string) bool {
	clean1 := strings.TrimPrefix(strings.TrimPrefix(img1, "docker.io/"), "library/")
	clean2 := strings.TrimPrefix(strings.TrimPrefix(img2, "docker.io/"), "library/")
	return clean1 == clean2
}

// ensureDaemonSetOnDelete guarantees that the envoy-proxy DaemonSet uses the OnDelete update strategy.
// This prevents Kubernetes daemonset-controller from automatically killing healthy pods during spec changes.
func (r *EnvoyGatewayRolloutReconciler) ensureDaemonSetOnDelete(ctx context.Context, targetNamespace string) error {
	ds := &appsv1.DaemonSet{}
	err := r.Get(ctx, client.ObjectKey{Namespace: targetNamespace, Name: "envoy-proxy"}, ds)
	if err != nil {
		return err
	}
	if ds.Spec.UpdateStrategy.Type != appsv1.OnDeleteDaemonSetStrategyType {
		ds.Spec.UpdateStrategy = appsv1.DaemonSetUpdateStrategy{
			Type: appsv1.OnDeleteDaemonSetStrategyType,
		}
		return r.Update(ctx, ds)
	}
	return nil
}

// resolveEnvoyImageForVersion determines the corresponding official Envoy proxy container image
// based on the Envoy Gateway release version.
func resolveEnvoyImageForVersion(gatewayVersion string) string {
	cleanVer := strings.TrimPrefix(gatewayVersion, "v")
	if strings.HasPrefix(cleanVer, "1.9.1") {
		return "docker.io/envoyproxy/envoy:distroless-v1.39.1"
	}
	if strings.HasPrefix(cleanVer, "1.9") {
		return "docker.io/envoyproxy/envoy:distroless-v1.39.0"
	}
	if strings.HasPrefix(cleanVer, "1.8") {
		return "docker.io/envoyproxy/envoy:distroless-v1.38.4"
	}
	if strings.HasPrefix(cleanVer, "1.2") {
		return "docker.io/envoyproxy/envoy:distroless-v1.32.3"
	}
	return "docker.io/envoyproxy/envoy:distroless-v1.39.0"
}
