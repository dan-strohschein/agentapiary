// Package api provides conversion functions between protobuf types and internal types.
//
// This file contains conversion functions for converting between:
//   - Protobuf types (from github.com/agentapiary/apiary/api/proto/v1)
//   - Internal types (from github.com/agentapiary/apiary/pkg/apiary)
//
// Key differences handled:
//   - Timestamps: protobuf uses *timestamppb.Timestamp, internal uses time.Time
//   - Pointers: protobuf uses pointers for nested structs, internal uses values
//   - Field types: some fields differ (int32 vs int, etc.)
package api

import (
	"encoding/json"
	"time"

	"github.com/agentapiary/apiary/api/proto/v1"
	"github.com/agentapiary/apiary/pkg/apiary"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Helper functions for timestamp conversion

// timestampToTime converts a protobuf timestamp to Go time.Time.
// Returns zero time if timestamp is nil.
func timestampToTime(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}

// timeToTimestamp converts a Go time.Time to protobuf timestamp.
// Returns nil if time is zero.
func timeToTimestamp(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

// Helper functions for TypeMeta conversion

// typeMetaToProto converts internal TypeMeta to protobuf TypeMeta.
func typeMetaToProto(tm apiary.TypeMeta) *v1.TypeMeta {
	if tm.APIVersion == "" && tm.Kind == "" {
		return nil
	}
	return &v1.TypeMeta{
		ApiVersion: tm.APIVersion,
		Kind:       tm.Kind,
	}
}

// typeMetaFromProto converts protobuf TypeMeta to internal TypeMeta.
func typeMetaFromProto(tm *v1.TypeMeta) apiary.TypeMeta {
	if tm == nil {
		return apiary.TypeMeta{}
	}
	return apiary.TypeMeta{
		APIVersion: tm.GetApiVersion(),
		Kind:       tm.GetKind(),
	}
}

// Helper functions for ObjectMeta conversion

// objectMetaToProto converts internal ObjectMeta to protobuf ObjectMeta.
func objectMetaToProto(om apiary.ObjectMeta) *v1.ObjectMeta {
	return &v1.ObjectMeta{
		Name:        om.Name,
		Namespace:   om.Namespace,
		Uid:         om.UID,
		Labels:      om.Labels, // Labels is map[string]string in both
		Annotations: om.Annotations,
		CreatedAt:   timeToTimestamp(om.CreatedAt),
		UpdatedAt:   timeToTimestamp(om.UpdatedAt),
	}
}

// objectMetaFromProto converts protobuf ObjectMeta to internal ObjectMeta.
func objectMetaFromProto(om *v1.ObjectMeta) apiary.ObjectMeta {
	if om == nil {
		return apiary.ObjectMeta{}
	}
	return apiary.ObjectMeta{
		Name:        om.GetName(),
		Namespace:   om.GetNamespace(),
		UID:         om.GetUid(),
		Labels:      om.GetLabels(), // Labels is map[string]string in both
		Annotations: om.GetAnnotations(),
		CreatedAt:   timestampToTime(om.GetCreatedAt()),
		UpdatedAt:   timestampToTime(om.GetUpdatedAt()),
	}
}

// Helper functions for ResourceRequirements conversion

// resourceRequirementsToProto converts internal ResourceRequirements to protobuf.
func resourceRequirementsToProto(rr apiary.ResourceRequirements) *v1.ResourceRequirements {
	return &v1.ResourceRequirements{
		Cpu:    rr.CPU,
		Memory: rr.Memory,
	}
}

// resourceRequirementsFromProto converts protobuf ResourceRequirements to internal.
func resourceRequirementsFromProto(rr *v1.ResourceRequirements) apiary.ResourceRequirements {
	if rr == nil {
		return apiary.ResourceRequirements{}
	}
	return apiary.ResourceRequirements{
		CPU:    rr.GetCpu(),
		Memory: rr.GetMemory(),
	}
}

// Helper functions for EnvVar conversion

// envVarToProto converts internal EnvVar to protobuf EnvVar.
func envVarToProto(ev apiary.EnvVar) *v1.EnvVar {
	result := &v1.EnvVar{
		Name:  ev.Name,
		Value: ev.Value,
	}
	if ev.ValueFrom != nil {
		result.ValueFrom = &v1.EnvVarSource{
			SecretKeyRef: &v1.SecretKeySelector{
				Name: ev.ValueFrom.SecretKeyRef.Name,
				Key:  ev.ValueFrom.SecretKeyRef.Key,
			},
		}
	}
	return result
}

// envVarFromProto converts protobuf EnvVar to internal EnvVar.
func envVarFromProto(ev *v1.EnvVar) apiary.EnvVar {
	if ev == nil {
		return apiary.EnvVar{}
	}
	result := apiary.EnvVar{
		Name:  ev.GetName(),
		Value: ev.GetValue(),
	}
	if ev.GetValueFrom() != nil && ev.GetValueFrom().GetSecretKeyRef() != nil {
		result.ValueFrom = &apiary.EnvVarSource{
			SecretKeyRef: &apiary.SecretKeySelector{
				Name: ev.GetValueFrom().GetSecretKeyRef().GetName(),
				Key:  ev.GetValueFrom().GetSecretKeyRef().GetKey(),
			},
		}
	}
	return result
}

// Helper functions for RuntimeConfig conversion

// runtimeConfigToProto converts internal RuntimeConfig to protobuf RuntimeConfig.
func runtimeConfigToProto(rc apiary.RuntimeConfig) *v1.RuntimeConfig {
	env := make([]*v1.EnvVar, len(rc.Env))
	for i, e := range rc.Env {
		env[i] = envVarToProto(e)
	}
	return &v1.RuntimeConfig{
		Command:    rc.Command,
		WorkingDir: rc.WorkingDir,
		Env:        env,
	}
}

// runtimeConfigFromProto converts protobuf RuntimeConfig to internal RuntimeConfig.
func runtimeConfigFromProto(rc *v1.RuntimeConfig) apiary.RuntimeConfig {
	if rc == nil {
		return apiary.RuntimeConfig{}
	}
	env := make([]apiary.EnvVar, len(rc.GetEnv()))
	for i, e := range rc.GetEnv() {
		env[i] = envVarFromProto(e)
	}
	return apiary.RuntimeConfig{
		Command:    rc.GetCommand(),
		WorkingDir: rc.GetWorkingDir(),
		Env:        env,
	}
}

// Helper functions for InterfaceConfig conversion

// interfaceConfigToProto converts internal InterfaceConfig to protobuf InterfaceConfig.
func interfaceConfigToProto(ic apiary.InterfaceConfig) *v1.InterfaceConfig {
	return &v1.InterfaceConfig{
		Type:        ic.Type,
		Port:        int32(ic.Port),
		HealthPath:  ic.HealthPath,
		MessagePath: ic.MessagePath,
	}
}

// interfaceConfigFromProto converts protobuf InterfaceConfig to internal InterfaceConfig.
func interfaceConfigFromProto(ic *v1.InterfaceConfig) apiary.InterfaceConfig {
	if ic == nil {
		return apiary.InterfaceConfig{}
	}
	return apiary.InterfaceConfig{
		Type:        ic.GetType(),
		Port:        int(ic.GetPort()),
		HealthPath:  ic.GetHealthPath(),
		MessagePath: ic.GetMessagePath(),
	}
}

// Helper functions for ResourceConfig conversion

// resourceConfigToProto converts internal ResourceConfig to protobuf ResourceConfig.
func resourceConfigToProto(rc apiary.ResourceConfig) *v1.ResourceConfig {
	return &v1.ResourceConfig{
		Requests:              resourceRequirementsToProto(rc.Requests),
		Limits:                 resourceRequirementsToProto(rc.Limits),
		TokenBudgetPerRequest: rc.TokenBudgetPerRequest,
	}
}

// resourceConfigFromProto converts protobuf ResourceConfig to internal ResourceConfig.
func resourceConfigFromProto(rc *v1.ResourceConfig) apiary.ResourceConfig {
	if rc == nil {
		return apiary.ResourceConfig{}
	}
	return apiary.ResourceConfig{
		Requests:              resourceRequirementsFromProto(rc.GetRequests()),
		Limits:                 resourceRequirementsFromProto(rc.GetLimits()),
		TokenBudgetPerRequest: rc.GetTokenBudgetPerRequest(),
	}
}

// Helper functions for ScalingConfig conversion

// scalingConfigToProto converts internal ScalingConfig to protobuf ScalingConfig.
func scalingConfigToProto(sc apiary.ScalingConfig) *v1.ScalingConfig {
	return &v1.ScalingConfig{
		MinReplicas:        int32(sc.MinReplicas),
		MaxReplicas:        int32(sc.MaxReplicas),
		TargetLatencyMs:    int32(sc.TargetLatencyMs),
		ScaleUpThreshold:   sc.ScaleUpThreshold,
		ScaleDownThreshold: sc.ScaleDownThreshold,
		CooldownSeconds:    int32(sc.CooldownSeconds),
	}
}

// scalingConfigFromProto converts protobuf ScalingConfig to internal ScalingConfig.
func scalingConfigFromProto(sc *v1.ScalingConfig) apiary.ScalingConfig {
	if sc == nil {
		return apiary.ScalingConfig{}
	}
	return apiary.ScalingConfig{
		MinReplicas:        int(sc.GetMinReplicas()),
		MaxReplicas:        int(sc.GetMaxReplicas()),
		TargetLatencyMs:    int(sc.GetTargetLatencyMs()),
		ScaleUpThreshold:   sc.GetScaleUpThreshold(),
		ScaleDownThreshold: sc.GetScaleDownThreshold(),
		CooldownSeconds:    int(sc.GetCooldownSeconds()),
	}
}

// Helper functions for GuardrailConfig conversion
// Note: OutputSchema is map[string]interface{} in internal, but string (JSON) in protobuf

// guardrailConfigToProto converts internal GuardrailConfig to protobuf GuardrailConfig.
func guardrailConfigToProto(gc apiary.GuardrailConfig) *v1.GuardrailConfig {
	var outputSchema string
	if gc.OutputSchema != nil {
		// Marshal to JSON string
		if data, err := json.Marshal(gc.OutputSchema); err == nil {
			outputSchema = string(data)
		}
	}
	return &v1.GuardrailConfig{
		ResponseTimeoutSeconds: int32(gc.ResponseTimeoutSeconds),
		RateLimitPerMinute:     int32(gc.RateLimitPerMinute),
		OutputSchema:           outputSchema,
		TokenBudgetPerRequest:  gc.TokenBudgetPerRequest,
		TokenBudgetPerSession:  gc.TokenBudgetPerSession,
		TokenBudgetPerMinute:   gc.TokenBudgetPerMinute,
	}
}

// guardrailConfigFromProto converts protobuf GuardrailConfig to internal GuardrailConfig.
func guardrailConfigFromProto(gc *v1.GuardrailConfig) apiary.GuardrailConfig {
	if gc == nil {
		return apiary.GuardrailConfig{}
	}
	var outputSchema map[string]interface{}
	if gc.GetOutputSchema() != "" {
		// Unmarshal from JSON string
		if err := json.Unmarshal([]byte(gc.GetOutputSchema()), &outputSchema); err != nil {
			outputSchema = nil
		}
	}
	return apiary.GuardrailConfig{
		ResponseTimeoutSeconds: int(gc.GetResponseTimeoutSeconds()),
		RateLimitPerMinute:     int(gc.GetRateLimitPerMinute()),
		OutputSchema:           outputSchema,
		TokenBudgetPerRequest:  gc.GetTokenBudgetPerRequest(),
		TokenBudgetPerSession:  gc.GetTokenBudgetPerSession(),
		TokenBudgetPerMinute:   gc.GetTokenBudgetPerMinute(),
	}
}

// Helper functions for AgentSpecSpec conversion

// agentSpecSpecToProto converts internal AgentSpecSpec to protobuf AgentSpecSpec.
func agentSpecSpecToProto(spec apiary.AgentSpecSpec) *v1.AgentSpecSpec {
	result := &v1.AgentSpecSpec{
		Runtime:    runtimeConfigToProto(spec.Runtime),
		Interface:  interfaceConfigToProto(spec.Interface),
		Resources:  resourceConfigToProto(spec.Resources),
		TaskTier:   spec.TaskTier,
	}
	if spec.Scaling.MinReplicas > 0 || spec.Scaling.MaxReplicas > 0 {
		result.Scaling = scalingConfigToProto(spec.Scaling)
	}
	if spec.Guardrails.ResponseTimeoutSeconds > 0 || spec.Guardrails.RateLimitPerMinute > 0 {
		result.Guardrails = guardrailConfigToProto(spec.Guardrails)
	}
	return result
}

// agentSpecSpecFromProto converts protobuf AgentSpecSpec to internal AgentSpecSpec.
func agentSpecSpecFromProto(spec *v1.AgentSpecSpec) apiary.AgentSpecSpec {
	if spec == nil {
		return apiary.AgentSpecSpec{}
	}
	result := apiary.AgentSpecSpec{
		Runtime:   runtimeConfigFromProto(spec.GetRuntime()),
		Interface: interfaceConfigFromProto(spec.GetInterface()),
		Resources: resourceConfigFromProto(spec.GetResources()),
		TaskTier:  spec.GetTaskTier(),
	}
	if spec.GetScaling() != nil {
		result.Scaling = scalingConfigFromProto(spec.GetScaling())
	}
	if spec.GetGuardrails() != nil {
		result.Guardrails = guardrailConfigFromProto(spec.GetGuardrails())
	}
	return result
}

// Helper functions for AgentSpec conversion

// agentSpecToProto converts internal AgentSpec to protobuf AgentSpec.
func agentSpecToProto(as *apiary.AgentSpec) *v1.AgentSpec {
	if as == nil {
		return nil
	}
	return &v1.AgentSpec{
		TypeMeta: typeMetaToProto(as.TypeMeta),
		Metadata: objectMetaToProto(as.ObjectMeta),
		Spec:     agentSpecSpecToProto(as.Spec),
	}
}

// agentSpecFromProto converts protobuf AgentSpec to internal AgentSpec.
func agentSpecFromProto(as *v1.AgentSpec) *apiary.AgentSpec {
	if as == nil {
		return nil
	}
	return &apiary.AgentSpec{
		TypeMeta:   typeMetaFromProto(as.GetTypeMeta()),
		ObjectMeta: objectMetaFromProto(as.GetMetadata()),
		Spec:       agentSpecSpecFromProto(as.GetSpec()),
	}
}

// Helper functions for HealthStatus conversion

// healthStatusToProto converts internal HealthStatus to protobuf HealthStatus.
func healthStatusToProto(hs *apiary.HealthStatus) *v1.HealthStatus {
	if hs == nil {
		return nil
	}
	// Convert Details from map[string]interface{} to map[string]string
	details := make(map[string]string)
	for k, v := range hs.Details {
		if str, ok := v.(string); ok {
			details[k] = str
		} else {
			// Convert non-string values to JSON
			if data, err := json.Marshal(v); err == nil {
				details[k] = string(data)
			}
		}
	}
	return &v1.HealthStatus{
		Healthy:   hs.Healthy,
		Message:   hs.Message,
		Timestamp: timeToTimestamp(hs.Timestamp),
		Details:   details,
	}
}

// healthStatusFromProto converts protobuf HealthStatus to internal HealthStatus.
func healthStatusFromProto(hs *v1.HealthStatus) *apiary.HealthStatus {
	if hs == nil {
		return nil
	}
	// Convert Details from map[string]string to map[string]interface{}
	details := make(map[string]interface{})
	for k, v := range hs.GetDetails() {
		// Try to unmarshal as JSON, otherwise use as string
		var val interface{}
		if err := json.Unmarshal([]byte(v), &val); err == nil {
			details[k] = val
		} else {
			details[k] = v
		}
	}
	return &apiary.HealthStatus{
		Healthy:   hs.GetHealthy(),
		Message:   hs.GetMessage(),
		Timestamp: timestampToTime(hs.GetTimestamp()),
		Details:   details,
	}
}

// Helper functions for DronePhase conversion

// dronePhaseToProto converts internal DronePhase (string) to protobuf DronePhase (enum).
func dronePhaseToProto(phase apiary.DronePhase) v1.DronePhase {
	switch phase {
	case apiary.DronePhasePending:
		return v1.DronePhase_DRONE_PHASE_PENDING
	case apiary.DronePhaseStarting:
		return v1.DronePhase_DRONE_PHASE_STARTING
	case apiary.DronePhaseRunning:
		return v1.DronePhase_DRONE_PHASE_RUNNING
	case apiary.DronePhaseStopping:
		return v1.DronePhase_DRONE_PHASE_STOPPING
	case apiary.DronePhaseStopped:
		return v1.DronePhase_DRONE_PHASE_STOPPED
	case apiary.DronePhaseFailed:
		return v1.DronePhase_DRONE_PHASE_FAILED
	default:
		return v1.DronePhase_DRONE_PHASE_UNSPECIFIED
	}
}

// dronePhaseFromProto converts protobuf DronePhase (enum) to internal DronePhase (string).
func dronePhaseFromProto(phase v1.DronePhase) apiary.DronePhase {
	switch phase {
	case v1.DronePhase_DRONE_PHASE_PENDING:
		return apiary.DronePhasePending
	case v1.DronePhase_DRONE_PHASE_STARTING:
		return apiary.DronePhaseStarting
	case v1.DronePhase_DRONE_PHASE_RUNNING:
		return apiary.DronePhaseRunning
	case v1.DronePhase_DRONE_PHASE_STOPPING:
		return apiary.DronePhaseStopping
	case v1.DronePhase_DRONE_PHASE_STOPPED:
		return apiary.DronePhaseStopped
	case v1.DronePhase_DRONE_PHASE_FAILED:
		return apiary.DronePhaseFailed
	default:
		return apiary.DronePhasePending
	}
}

// Helper functions for DroneStatus conversion

// droneStatusToProto converts internal DroneStatus to protobuf DroneStatus.
func droneStatusToProto(ds apiary.DroneStatus) *v1.DroneStatus {
	result := &v1.DroneStatus{
		Phase:      dronePhaseToProto(ds.Phase),
		Message:    ds.Message,
		KeeperAddr: ds.KeeperAddr,
		Health:     healthStatusToProto(ds.Health),
	}
	if ds.StartedAt != nil {
		result.StartedAt = timeToTimestamp(*ds.StartedAt)
	}
	return result
}

// droneStatusFromProto converts protobuf DroneStatus to internal DroneStatus.
func droneStatusFromProto(ds *v1.DroneStatus) apiary.DroneStatus {
	if ds == nil {
		return apiary.DroneStatus{}
	}
	result := apiary.DroneStatus{
		Phase:      dronePhaseFromProto(ds.GetPhase()),
		Message:    ds.GetMessage(),
		KeeperAddr: ds.GetKeeperAddr(),
		Health:     healthStatusFromProto(ds.GetHealth()),
	}
	if ds.GetStartedAt() != nil {
		t := timestampToTime(ds.GetStartedAt())
		result.StartedAt = &t
	}
	return result
}

// Helper functions for Drone conversion

// droneToProto converts internal Drone to protobuf Drone.
func droneToProto(d *apiary.Drone) *v1.Drone {
	if d == nil {
		return nil
	}
	result := &v1.Drone{
		TypeMeta: typeMetaToProto(d.TypeMeta),
		Metadata: objectMetaToProto(d.ObjectMeta),
		Status:   droneStatusToProto(d.Status),
	}
	if d.Spec != nil {
		result.Spec = agentSpecToProto(d.Spec)
	}
	return result
}

// droneFromProto converts protobuf Drone to internal Drone.
func droneFromProto(d *v1.Drone) *apiary.Drone {
	if d == nil {
		return nil
	}
	result := &apiary.Drone{
		TypeMeta:   typeMetaFromProto(d.GetTypeMeta()),
		ObjectMeta: objectMetaFromProto(d.GetMetadata()),
		Status:     droneStatusFromProto(d.GetStatus()),
	}
	if d.GetSpec() != nil {
		result.Spec = agentSpecFromProto(d.GetSpec())
	}
	return result
}

// Helper functions for RetryPolicy conversion

// retryPolicyToProto converts internal RetryPolicy to protobuf RetryPolicy.
func retryPolicyToProto(rp apiary.RetryPolicy) *v1.RetryPolicy {
	return &v1.RetryPolicy{
		MaxRetries:        int32(rp.MaxRetries),
		BackoffMultiplier: int32(rp.BackoffMultiplier),
		InitialDelayMs:    int32(rp.InitialDelayMs),
	}
}

// retryPolicyFromProto converts protobuf RetryPolicy to internal RetryPolicy.
func retryPolicyFromProto(rp *v1.RetryPolicy) apiary.RetryPolicy {
	if rp == nil {
		return apiary.RetryPolicy{}
	}
	return apiary.RetryPolicy{
		MaxRetries:        int(rp.GetMaxRetries()),
		BackoffMultiplier: int(rp.GetBackoffMultiplier()),
		InitialDelayMs:    int(rp.GetInitialDelayMs()),
	}
}

// Helper functions for SessionConfig conversion

// sessionConfigToProto converts internal SessionConfig to protobuf SessionConfig.
func sessionConfigToProto(sc apiary.SessionConfig) *v1.SessionConfig {
	return &v1.SessionConfig{
		TimeoutMinutes:     int32(sc.TimeoutMinutes),
		MaxDurationMinutes: int32(sc.MaxDurationMinutes),
		PersistOnTerminate: sc.PersistOnTerminate,
		PersistPath:        sc.PersistPath,
		MaxMemoryMb:        int32(sc.MaxMemoryMB),
	}
}

// sessionConfigFromProto converts protobuf SessionConfig to internal SessionConfig.
func sessionConfigFromProto(sc *v1.SessionConfig) apiary.SessionConfig {
	if sc == nil {
		return apiary.SessionConfig{}
	}
	return apiary.SessionConfig{
		TimeoutMinutes:     int(sc.GetTimeoutMinutes()),
		MaxDurationMinutes: int(sc.GetMaxDurationMinutes()),
		PersistOnTerminate: sc.GetPersistOnTerminate(),
		PersistPath:        sc.GetPersistPath(),
		MaxMemoryMB:        int(sc.GetMaxMemoryMb()),
	}
}

// Helper functions for Stage conversion

// stageToProto converts internal Stage to protobuf Stage.
func stageToProto(s apiary.Stage) *v1.Stage {
	return &v1.Stage{
		Name:     s.Name,
		AgentRef: s.AgentRef,
	}
}

// stageFromProto converts protobuf Stage to internal Stage.
func stageFromProto(s *v1.Stage) apiary.Stage {
	if s == nil {
		return apiary.Stage{}
	}
	return apiary.Stage{
		Name:     s.GetName(),
		AgentRef: s.GetAgentRef(),
	}
}

// Helper functions for RoutingConfig conversion

// routingConfigToProto converts internal RoutingConfig to protobuf RoutingConfig.
func routingConfigToProto(rc apiary.RoutingConfig) *v1.RoutingConfig {
	result := &v1.RoutingConfig{
		DeadLetterTopic: rc.DeadLetterTopic,
	}
	if rc.RetryPolicy.MaxRetries > 0 {
		result.RetryPolicy = retryPolicyToProto(rc.RetryPolicy)
	}
	return result
}

// routingConfigFromProto converts protobuf RoutingConfig to internal RoutingConfig.
func routingConfigFromProto(rc *v1.RoutingConfig) apiary.RoutingConfig {
	if rc == nil {
		return apiary.RoutingConfig{}
	}
	result := apiary.RoutingConfig{
		DeadLetterTopic: rc.GetDeadLetterTopic(),
	}
	if rc.GetRetryPolicy() != nil {
		result.RetryPolicy = retryPolicyFromProto(rc.GetRetryPolicy())
	}
	return result
}

// Helper functions for WebhookConfig conversion

// webhookConfigToProto converts internal WebhookConfig to protobuf WebhookConfig.
func webhookConfigToProto(wc apiary.WebhookConfig) *v1.WebhookConfig {
	return &v1.WebhookConfig{
		Url:        wc.URL,
		AuthHeader: wc.AuthHeader,
	}
}

// webhookConfigFromProto converts protobuf WebhookConfig to internal WebhookConfig.
func webhookConfigFromProto(wc *v1.WebhookConfig) apiary.WebhookConfig {
	if wc == nil {
		return apiary.WebhookConfig{}
	}
	return apiary.WebhookConfig{
		URL:        wc.GetUrl(),
		AuthHeader: wc.GetAuthHeader(),
	}
}

// Helper functions for HiveSpec conversion

// hiveSpecToProto converts internal HiveSpec to protobuf HiveSpec.
func hiveSpecToProto(hs apiary.HiveSpec) *v1.HiveSpec {
	result := &v1.HiveSpec{
		PoolMode:     hs.PoolMode,
		WarmPoolSize: int32(hs.WarmPoolSize),
		Pattern:      hs.Pattern,
	}
	if hs.Session.TimeoutMinutes > 0 || hs.Session.MaxDurationMinutes > 0 {
		result.Session = sessionConfigToProto(hs.Session)
	}
	if len(hs.Stages) > 0 {
		stages := make([]*v1.Stage, len(hs.Stages))
		for i, s := range hs.Stages {
			stages[i] = stageToProto(s)
		}
		result.Stages = stages
	}
	if hs.Routing.DeadLetterTopic != "" || hs.Routing.RetryPolicy.MaxRetries > 0 {
		result.Routing = routingConfigToProto(hs.Routing)
	}
	if hs.Guardrails.ResponseTimeoutSeconds > 0 || hs.Guardrails.RateLimitPerMinute > 0 {
		result.Guardrails = guardrailConfigToProto(hs.Guardrails)
	}
	if hs.Webhook.URL != "" {
		result.Webhook = webhookConfigToProto(hs.Webhook)
	}
	return result
}

// hiveSpecFromProto converts protobuf HiveSpec to internal HiveSpec.
func hiveSpecFromProto(hs *v1.HiveSpec) apiary.HiveSpec {
	if hs == nil {
		return apiary.HiveSpec{}
	}
	result := apiary.HiveSpec{
		PoolMode:     hs.GetPoolMode(),
		WarmPoolSize: int(hs.GetWarmPoolSize()),
		Pattern:      hs.GetPattern(),
	}
	if hs.GetSession() != nil {
		result.Session = sessionConfigFromProto(hs.GetSession())
	}
	if len(hs.GetStages()) > 0 {
		stages := make([]apiary.Stage, len(hs.GetStages()))
		for i, s := range hs.GetStages() {
			stages[i] = stageFromProto(s)
		}
		result.Stages = stages
	}
	if hs.GetRouting() != nil {
		result.Routing = routingConfigFromProto(hs.GetRouting())
	}
	if hs.GetGuardrails() != nil {
		result.Guardrails = guardrailConfigFromProto(hs.GetGuardrails())
	}
	if hs.GetWebhook() != nil {
		result.Webhook = webhookConfigFromProto(hs.GetWebhook())
	}
	return result
}

// Helper functions for HivePhase conversion

// hivePhaseToProto converts internal HivePhase (string) to protobuf HivePhase (enum).
func hivePhaseToProto(phase apiary.HivePhase) v1.HivePhase {
	switch phase {
	case apiary.HivePhasePending:
		return v1.HivePhase_HIVE_PHASE_PENDING
	case apiary.HivePhaseActive:
		return v1.HivePhase_HIVE_PHASE_ACTIVE
	case apiary.HivePhasePaused:
		return v1.HivePhase_HIVE_PHASE_PAUSED
	case apiary.HivePhaseFailed:
		return v1.HivePhase_HIVE_PHASE_FAILED
	default:
		return v1.HivePhase_HIVE_PHASE_UNSPECIFIED
	}
}

// hivePhaseFromProto converts protobuf HivePhase (enum) to internal HivePhase (string).
func hivePhaseFromProto(phase v1.HivePhase) apiary.HivePhase {
	switch phase {
	case v1.HivePhase_HIVE_PHASE_PENDING:
		return apiary.HivePhasePending
	case v1.HivePhase_HIVE_PHASE_ACTIVE:
		return apiary.HivePhaseActive
	case v1.HivePhase_HIVE_PHASE_PAUSED:
		return apiary.HivePhasePaused
	case v1.HivePhase_HIVE_PHASE_FAILED:
		return apiary.HivePhaseFailed
	default:
		return apiary.HivePhasePending
	}
}

// Helper functions for HiveStatus conversion

// hiveStatusToProto converts internal HiveStatus to protobuf HiveStatus.
func hiveStatusToProto(hs apiary.HiveStatus) *v1.HiveStatus {
	return &v1.HiveStatus{
		Phase:        hivePhaseToProto(hs.Phase),
		Message:      hs.Message,
		ActiveDrones: int32(hs.ActiveDrones),
	}
}

// hiveStatusFromProto converts protobuf HiveStatus to internal HiveStatus.
func hiveStatusFromProto(hs *v1.HiveStatus) apiary.HiveStatus {
	if hs == nil {
		return apiary.HiveStatus{}
	}
	return apiary.HiveStatus{
		Phase:       hivePhaseFromProto(hs.GetPhase()),
		Message:     hs.GetMessage(),
		ActiveDrones: int(hs.GetActiveDrones()),
	}
}

// Helper functions for Hive conversion

// hiveToProto converts internal Hive to protobuf Hive.
func hiveToProto(h *apiary.Hive) *v1.Hive {
	if h == nil {
		return nil
	}
	result := &v1.Hive{
		TypeMeta: typeMetaToProto(h.TypeMeta),
		Metadata: objectMetaToProto(h.ObjectMeta),
		Spec:     hiveSpecToProto(h.Spec),
	}
	if h.Status.Phase != "" {
		result.Status = hiveStatusToProto(h.Status)
	}
	return result
}

// hiveFromProto converts protobuf Hive to internal Hive.
func hiveFromProto(h *v1.Hive) *apiary.Hive {
	if h == nil {
		return nil
	}
	result := &apiary.Hive{
		TypeMeta:   typeMetaFromProto(h.GetTypeMeta()),
		ObjectMeta: objectMetaFromProto(h.GetMetadata()),
		Spec:       hiveSpecFromProto(h.GetSpec()),
	}
	if h.GetStatus() != nil {
		result.Status = hiveStatusFromProto(h.GetStatus())
	}
	return result
}

// Helper functions for ResourceQuota conversion

// resourceQuotaToProto converts internal ResourceQuota to protobuf ResourceQuota.
func resourceQuotaToProto(rq apiary.ResourceQuota) *v1.ResourceQuota {
	return &v1.ResourceQuota{
		MaxHives:        int32(rq.MaxHives),
		MaxDronesPerHive: int32(rq.MaxDronesPerHive),
		MaxTotalDrones:  int32(rq.MaxTotalDrones),
		MaxMemoryGb:     int32(rq.MaxMemoryGB),
	}
}

// resourceQuotaFromProto converts protobuf ResourceQuota to internal ResourceQuota.
func resourceQuotaFromProto(rq *v1.ResourceQuota) apiary.ResourceQuota {
	if rq == nil {
		return apiary.ResourceQuota{}
	}
	return apiary.ResourceQuota{
		MaxHives:        int(rq.GetMaxHives()),
		MaxDronesPerHive: int(rq.GetMaxDronesPerHive()),
		MaxTotalDrones:  int(rq.GetMaxTotalDrones()),
		MaxMemoryGB:     int(rq.GetMaxMemoryGb()),
	}
}

// Helper functions for DefaultConfig conversion

// defaultConfigToProto converts internal DefaultConfig to protobuf DefaultConfig.
func defaultConfigToProto(dc apiary.DefaultConfig) *v1.DefaultConfig {
	result := &v1.DefaultConfig{}
	if dc.Guardrails.ResponseTimeoutSeconds > 0 || dc.Guardrails.RateLimitPerMinute > 0 {
		result.Guardrails = guardrailConfigToProto(dc.Guardrails)
	}
	return result
}

// defaultConfigFromProto converts protobuf DefaultConfig to internal DefaultConfig.
func defaultConfigFromProto(dc *v1.DefaultConfig) apiary.DefaultConfig {
	if dc == nil {
		return apiary.DefaultConfig{}
	}
	result := apiary.DefaultConfig{}
	if dc.GetGuardrails() != nil {
		result.Guardrails = guardrailConfigFromProto(dc.GetGuardrails())
	}
	return result
}

// Helper functions for CellSpec conversion

// cellSpecToProto converts internal CellSpec to protobuf CellSpec.
func cellSpecToProto(cs apiary.CellSpec) *v1.CellSpec {
	result := &v1.CellSpec{}
	if cs.ResourceQuota.MaxHives > 0 || cs.ResourceQuota.MaxTotalDrones > 0 {
		result.ResourceQuota = resourceQuotaToProto(cs.ResourceQuota)
	}
	if cs.Defaults.Guardrails.ResponseTimeoutSeconds > 0 || cs.Defaults.Guardrails.RateLimitPerMinute > 0 {
		result.Defaults = defaultConfigToProto(cs.Defaults)
	}
	return result
}

// cellSpecFromProto converts protobuf CellSpec to internal CellSpec.
func cellSpecFromProto(cs *v1.CellSpec) apiary.CellSpec {
	if cs == nil {
		return apiary.CellSpec{}
	}
	result := apiary.CellSpec{}
	if cs.GetResourceQuota() != nil {
		result.ResourceQuota = resourceQuotaFromProto(cs.GetResourceQuota())
	}
	if cs.GetDefaults() != nil {
		result.Defaults = defaultConfigFromProto(cs.GetDefaults())
	}
	return result
}

// Helper functions for Cell conversion

// cellToProto converts internal Cell to protobuf Cell.
func cellToProto(c *apiary.Cell) *v1.Cell {
	if c == nil {
		return nil
	}
	return &v1.Cell{
		TypeMeta: typeMetaToProto(c.TypeMeta),
		Metadata: objectMetaToProto(c.ObjectMeta),
		Spec:     cellSpecToProto(c.Spec),
	}
}

// cellFromProto converts protobuf Cell to internal Cell.
func cellFromProto(c *v1.Cell) *apiary.Cell {
	if c == nil {
		return nil
	}
	return &apiary.Cell{
		TypeMeta:   typeMetaFromProto(c.GetTypeMeta()),
		ObjectMeta: objectMetaFromProto(c.GetMetadata()),
		Spec:       cellSpecFromProto(c.GetSpec()),
	}
}

// Helper functions for Secret conversion

// secretToProto converts internal Secret to protobuf Secret.
func secretToProto(s *apiary.Secret) *v1.Secret {
	if s == nil {
		return nil
	}
	return &v1.Secret{
		TypeMeta: typeMetaToProto(s.TypeMeta),
		Metadata: objectMetaToProto(s.ObjectMeta),
		Data:     s.Data, // map[string][]byte is the same in both
	}
}

// secretFromProto converts protobuf Secret to internal Secret.
func secretFromProto(s *v1.Secret) *apiary.Secret {
	if s == nil {
		return nil
	}
	return &apiary.Secret{
		TypeMeta:   typeMetaFromProto(s.GetTypeMeta()),
		ObjectMeta: objectMetaFromProto(s.GetMetadata()),
		Data:       s.GetData(), // map[string][]byte is the same in both
	}
}
