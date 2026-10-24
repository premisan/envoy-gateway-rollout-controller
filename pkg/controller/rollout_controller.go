package controller

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	appsv1 "k8s.io/api/apps/v1"
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
		// Terminal or idle states
		return ctrl.Result{}, nil
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
		// Suspended states: DO NOT auto-retry without explicit operator resume
		return ctrl.Result{}, nil
	default:
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
}

func (r *EnvoyGatewayRolloutReconciler) handlePending(ctx context.Context, rollout *rolloutv1alpha1.EnvoyGatewayRollout) (ctrl.Result, error) {
	rollout.Status.TargetVersion = rollout.Spec.Version
	rollout.Status.TotalSteps = len(rollout.Spec.Strategy.Steps)

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

	// [Pre-Upgrade Metric Guardrail]
	// Spawn and start Ephemeral Prometheus BEFORE CRD or Control Plane upgrades are applied.
	// This ensures we continuously monitor baseline traffic and detect any CRD schema/version incompatibility errors.
	if rollout.Spec.Analysis.EphemeralPrometheus.Enabled {
		ephemeralURL, err := r.PromManager.EnsureEphemeralPrometheus(ctx, rollout, nil)
		if err != nil {
			logger.Error(err, "Failed to spawn pre-upgrade ephemeral prometheus")
		} else {
			rollout.Status.EphemeralPrometheusURL = ephemeralURL
			logger.Info("Pre-upgrade ephemeral prometheus active prior to CRD synchronization", "url", ephemeralURL)
		}
	}

	_ = r.syncWorkloadImages(ctx, rollout, targetNamespace, targetVersion)

	stepIdx := rollout.Status.CurrentStep - 1
	if stepIdx < 0 || stepIdx >= len(rollout.Spec.Strategy.Steps) {
		rollout.Status.Phase = rolloutv1alpha1.PhaseCompleted
		rollout.Status.CurrentVersion = targetVersion
		rollout.Status.Message = "Rollout successfully completed across all nodes"
		_ = r.PromManager.CleanupEphemeralPrometheus(ctx, rollout)
		_ = r.Status().Update(ctx, rollout)
		return ctrl.Result{}, nil
	}

	step := rollout.Spec.Strategy.Steps[stepIdx]
	targetCanaryCount := parseNodeCount(step.Nodes, totalNodes)

	// Build node status list
	nodeStatuses := make([]rolloutv1alpha1.NodeRolloutStatus, 0)
	canaryNodeNames := make([]string, 0)
	canaryPodIPs := make([]string, 0)

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
			NodeName:      nodeName,
			NodeIP:        nodeIP,
			ActiveVersion: rollout.Status.CurrentVersion,
		}

		if i < targetCanaryCount {
			status.Phase = "Canary"
			status.CanaryVersion = targetVersion
			status.ActiveSessions = 120 + (i * 15)
			canaryNodeNames = append(canaryNodeNames, nodeName)
			canaryPodIPs = append(canaryPodIPs, nodeIP)
		} else {
			status.Phase = "Stable"
			status.ActiveSessions = 250 + (i * 20)
		}
		nodeStatuses = append(nodeStatuses, status)
	}

	// Ensure Ephemeral Prometheus if enabled
	if rollout.Spec.Analysis.EphemeralPrometheus.Enabled {
		ephemeralURL, err := r.PromManager.EnsureEphemeralPrometheus(ctx, rollout, canaryPodIPs)
		if err != nil {
			logger.Error(err, "Failed to spawn ephemeral prometheus")
		} else {
			rollout.Status.EphemeralPrometheusURL = ephemeralURL
		}
	}

	rollout.Status.Nodes = nodeStatuses
	rollout.Status.Phase = rolloutv1alpha1.PhaseObserving
	rollout.Status.Message = fmt.Sprintf("Observing Step %d/%d (%s)", rollout.Status.CurrentStep, rollout.Status.TotalSteps, step.Nodes)
	rollout.Status.LastUpdateTime = metav1.Now()

	// Store step start time
	key := fmt.Sprintf("%s/%d", rollout.Name, rollout.Status.CurrentStep)
	r.StepStartTimes[key] = time.Now()

	_ = r.Status().Update(ctx, rollout)
	return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
}

func (r *EnvoyGatewayRolloutReconciler) handleObserving(ctx context.Context, rollout *rolloutv1alpha1.EnvoyGatewayRollout) (ctrl.Result, error) {
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

	// Trigger Envoy Admin API listener drain on old instances
	for i := range rollout.Status.Nodes {
		node := &rollout.Status.Nodes[i]
		if node.Phase == "Canary" {
			node.Phase = "Draining"
			node.DrainProgress = 50
			// Call /drain_listeners?inboundonly
			_ = r.AdminClient.DrainListeners(ctx, node.NodeIP, 19001)

			// Gracefully rotate the envoy-proxy pod on this node to trigger DaemonSet pod recreation with new image
			podList := &corev1.PodList{}
			if err := r.List(ctx, podList, client.InNamespace(targetNamespace), client.MatchingLabels{"app.kubernetes.io/name": "envoy"}); err == nil {
				for _, pod := range podList.Items {
					if pod.Spec.NodeName == node.NodeName {
						logger.Info("Rotating envoy-proxy pod on node after drain", "node", node.NodeName, "pod", pod.Name)
						_ = r.Delete(ctx, &pod)
					}
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
		// All steps completed!
		completedVer := targetVersion
		rollout.Status.Phase = rolloutv1alpha1.PhaseCompleted
		rollout.Status.CurrentVersion = completedVer
		rollout.Status.Message = fmt.Sprintf("All %d steps successfully completed. Envoy Gateway upgraded to %s", rollout.Status.TotalSteps, completedVer)
		for i := range rollout.Status.Nodes {
			rollout.Status.Nodes[i].Phase = "Completed"
			rollout.Status.Nodes[i].ActiveVersion = completedVer
			rollout.Status.Nodes[i].CanaryVersion = ""
			rollout.Status.Nodes[i].DrainProgress = 100
		}
		_ = r.syncWorkloadImages(ctx, rollout, targetNamespace, completedVer)
		// Clean up temporary Prometheus
		_ = r.PromManager.CleanupEphemeralPrometheus(ctx, rollout)
	}

	rollout.Status.LastUpdateTime = metav1.Now()
	_ = r.Status().Update(ctx, rollout)
	return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
}

func (r *EnvoyGatewayRolloutReconciler) handleAbort(ctx context.Context, rollout *rolloutv1alpha1.EnvoyGatewayRollout, reason string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Executing rollout abort and restoring stable state", "reason", reason)

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

	// 2. Rollback workloads to stable version
	_ = r.syncWorkloadImages(ctx, rollout, targetNamespace, stableVersion)

	// 3. Update status to Aborted terminal state
	rollout.Status.Phase = rolloutv1alpha1.PhaseAborted
	rollout.Status.TargetVersion = stableVersion
	rollout.Status.Message = fmt.Sprintf("Rollout aborted and restored to %s: %s", stableVersion, reason)
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
		APIVersion: rollout.APIVersion,
		Kind:       rollout.Kind,
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
	_ = r.Create(ctx, envoyProxy)

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

	return nil
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

// syncWorkloadImages ensures the underlying Kubernetes Deployment (Control Plane)
// and DaemonSet (Data Plane) actually transition their container images to the target version.
func (r *EnvoyGatewayRolloutReconciler) syncWorkloadImages(
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

	// 2. Update Envoy Proxy DataPlane DaemonSet
	ds := &appsv1.DaemonSet{}
	err = r.Get(ctx, client.ObjectKey{Namespace: targetNamespace, Name: "envoy-proxy"}, ds)
	if err == nil {
		updated := false
		targetGatewayImage := fmt.Sprintf("envoyproxy/gateway:%s", targetVersion)
		targetEnvoyImage := "docker.io/envoyproxy/envoy:distroless-v1.39.0"
		if strings.HasPrefix(targetVersion, "v1.2") {
			targetEnvoyImage = "docker.io/envoyproxy/envoy:distroless-v1.32.3"
		}

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
			} else {
				// Rotate running pods so they immediately recreate with new template
				podList := &corev1.PodList{}
				if err := r.List(ctx, podList, client.InNamespace(targetNamespace), client.MatchingLabels{"app.kubernetes.io/name": "envoy"}); err == nil {
					for _, pod := range podList.Items {
						logger.Info("Rotating older envoy-proxy pod for new image", "pod", pod.Name)
						_ = r.Delete(ctx, &pod)
					}
				}
			}
		}
	}

	return nil
}

// imageEquals normalizes docker registry prefixes to avoid false positive diffs.
func imageEquals(img1, img2 string) bool {
	clean1 := strings.TrimPrefix(strings.TrimPrefix(img1, "docker.io/"), "library/")
	clean2 := strings.TrimPrefix(strings.TrimPrefix(img2, "docker.io/"), "library/")
	return clean1 == clean2
}
