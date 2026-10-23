-- ArgoCD Custom Health Check for EnvoyGatewayRollout (rollout.envoyproxy.io/EnvoyGatewayRollout)
-- Put this script into argocd-cm ConfigMap under resource.customizations.health.rollout.envoyproxy.io_EnvoyGatewayRollout
local hs = {}

if obj.status ~= nil then
  local phase = obj.status.phase
  local msg = obj.status.message or ""

  if phase == "Completed" then
    hs.status = "Healthy"
    hs.message = "Envoy Gateway rollout completed successfully: " .. msg
    return hs
  elseif phase == "Aborted" or phase == "RollingBack" then
    hs.status = "Degraded"
    hs.message = "Envoy Gateway rollout degraded: " .. msg
    return hs
  elseif phase == "Paused" then
    hs.status = "Suspended"
    hs.message = "Envoy Gateway rollout paused by operator: " .. msg
    return hs
  elseif phase == "Observing" or phase == "CanaryRunning" or phase == "Deploying" or phase == "Draining" then
    hs.status = "Progressing"
    local step = tostring(obj.status.currentStep or 1)
    local total = tostring(obj.status.totalSteps or 1)
    local remaining = tostring(obj.status.stepTimeRemaining or "")
    if remaining ~= "" then
      hs.message = "Step " .. step .. "/" .. total .. " (" .. remaining .. ") - " .. tostring(msg)
    else
      hs.message = "Step " .. step .. "/" .. total .. " - " .. tostring(msg)
    end
    return hs
  end
end

hs.status = "Progressing"
hs.message = "Initializing rollout controller..."
return hs
