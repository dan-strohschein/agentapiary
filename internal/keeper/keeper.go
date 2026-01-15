// Package keeper provides the Keeper sidecar for managing agent processes.
package keeper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/agentapiary/apiary/internal/audit"
	"github.com/agentapiary/apiary/internal/comb"
	"github.com/agentapiary/apiary/internal/guardrails"
	"github.com/agentapiary/apiary/internal/metrics"
	"github.com/agentapiary/apiary/internal/nectar"
	"github.com/agentapiary/apiary/internal/webhook"
	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/agentapiary/apiary/pkg/waggle"
	"go.uber.org/zap"
)

// Keeper manages an agent process and provides platform services.
type Keeper struct {
	spec           *apiary.AgentSpec
	process        *exec.Cmd
	stdinPipe      io.WriteCloser
	logger         *zap.Logger
	store          apiary.ResourceStore
	health         *HealthStatus
	mu             sync.RWMutex
	server         *http.Server
	comb           comb.Comb
	sessionID      string
	cache          *combCache
	messageBus     nectar.MessageBus
	agentID        string
	busSubscription nectar.Subscription
	inboundQueue   *messageQueue
	outboundQueue  *messageQueue
	metrics         *metrics.Collector
	metricsServer   *http.Server
	tokenTracker    *guardrails.TokenBudgetTracker
	timeoutTracker  *guardrails.TimeoutTracker
	outputValidator *guardrails.OutputValidator
	rateLimiter     *guardrails.RateLimiter
	hiveCache       *hiveCache // Cache for SessionID -> HiveName mapping
	webhookMgr      *webhook.Manager // Webhook manager for guardrail violations
	auditLogger     *audit.Logger // Audit logger for structured logging
	relayCancel     context.CancelFunc // Cancel function for message relay goroutines
	relayWg          sync.WaitGroup    // WaitGroup to track message relay goroutines
}

// combCache provides local caching for Comb operations.
type combCache struct {
	data      map[string]*cacheEntry
	mu        sync.RWMutex
	maxSize   int
	insertOrder []string // Track insertion order for FIFO
}

type cacheEntry struct {
	value       string
	expiresAt   time.Time
	insertedAt  time.Time
}

func newCombCache(maxSize int) *combCache {
	return &combCache{
		data:        make(map[string]*cacheEntry),
		maxSize:     maxSize,
		insertOrder: make([]string, 0, maxSize),
	}
}

func (c *combCache) get(key string) (string, bool) {
	c.mu.RLock()
	entry, exists := c.data[key]
	if !exists {
		c.mu.RUnlock()
		return "", false
	}
	
	expired := time.Now().After(entry.expiresAt)
	c.mu.RUnlock()
	
	if expired {
		// Need write lock to delete
		c.mu.Lock()
		// Re-check entry still exists and is expired
		if entry, exists := c.data[key]; exists && time.Now().After(entry.expiresAt) {
			delete(c.data, key)
			// Remove from insertion order
			for i, k := range c.insertOrder {
				if k == key {
					c.insertOrder = append(c.insertOrder[:i], c.insertOrder[i+1:]...)
					break
				}
			}
		}
		c.mu.Unlock()
		return "", false
	}
	
	return entry.value, true
}

func (c *combCache) set(key, value string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	now := time.Now()
	expiresAt := now.Add(ttl)
	if ttl == 0 {
		expiresAt = now.Add(5 * time.Minute) // Default cache TTL
	}
	
	// Check if key already exists
	_, exists := c.data[key]
	
	// Simple eviction: if cache is full and key is new, remove oldest (FIFO)
	if !exists && len(c.data) >= c.maxSize {
		// Remove oldest entry (first in insertion order)
		if len(c.insertOrder) > 0 {
			oldestKey := c.insertOrder[0]
			delete(c.data, oldestKey)
			c.insertOrder = c.insertOrder[1:]
		}
	} else if exists {
		// Remove from insertion order to re-add at end (update access order)
		for i, k := range c.insertOrder {
			if k == key {
				c.insertOrder = append(c.insertOrder[:i], c.insertOrder[i+1:]...)
				break
			}
		}
	}
	
	c.data[key] = &cacheEntry{
		value:      value,
		expiresAt:  expiresAt,
		insertedAt: now,
	}
	
	// Add to end of insertion order (if new)
	if !exists {
		c.insertOrder = append(c.insertOrder, key)
	}
}

func (c *combCache) invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
	// Remove from insertion order
	for i, k := range c.insertOrder {
		if k == key {
			c.insertOrder = append(c.insertOrder[:i], c.insertOrder[i+1:]...)
			break
		}
	}
}

func (c *combCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = make(map[string]*cacheEntry)
	c.insertOrder = make([]string, 0, c.maxSize)
}

// hiveCache provides caching for SessionID -> HiveName mapping.
type hiveCache struct {
	data    map[string]string // SessionID -> HiveName
	mu      sync.RWMutex
	maxSize int
}

func newHiveCache(maxSize int) *hiveCache {
	return &hiveCache{
		data:    make(map[string]string),
		maxSize: maxSize,
	}
}

func (c *hiveCache) get(sessionID string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	hiveName, exists := c.data[sessionID]
	return hiveName, exists
}

func (c *hiveCache) set(sessionID, hiveName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Simple eviction: if cache is full, remove a random entry
	// In practice, this is unlikely to be an issue since cache is per-Keeper
	if len(c.data) >= c.maxSize && len(c.data) > 0 {
		// Remove first entry (simple eviction)
		for k := range c.data {
			delete(c.data, k)
			break
		}
	}
	c.data[sessionID] = hiveName
}

// HealthStatus represents the health of the agent.
type HealthStatus struct {
	Healthy   bool      `json:"healthy"`
	Message   string    `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// NewKeeper creates a new Keeper instance.
func NewKeeper(spec *apiary.AgentSpec, logger *zap.Logger, store apiary.ResourceStore, combStore comb.Comb, sessionID string, webhookMgr *webhook.Manager) *Keeper {
	metricsCollector := metrics.NewCollector(logger)

	// Initialize token budget tracker from guardrails config
	var tokenTracker *guardrails.TokenBudgetTracker
	var timeoutTracker *guardrails.TimeoutTracker
	var outputValidator *guardrails.OutputValidator
	var rateLimiter *guardrails.RateLimiter
	if spec != nil {
		guardrailsCfg := spec.Spec.Guardrails
		// Also check ResourceConfig for per-request budget (legacy field)
		perRequest := guardrailsCfg.TokenBudgetPerRequest
		if perRequest == 0 && spec.Spec.Resources.TokenBudgetPerRequest > 0 {
			perRequest = spec.Spec.Resources.TokenBudgetPerRequest
		}
		perSession := guardrailsCfg.TokenBudgetPerSession
		perMinute := guardrailsCfg.TokenBudgetPerMinute

		// Only create tracker if at least one budget is configured
		if perRequest > 0 || perSession > 0 || perMinute > 0 {
			tokenTracker = guardrails.NewTokenBudgetTracker(perRequest, perSession, perMinute)
		}

		// Initialize timeout tracker from guardrails config
		if guardrailsCfg.ResponseTimeoutSeconds > 0 {
			timeoutTracker = guardrails.NewTimeoutTracker(guardrailsCfg.ResponseTimeoutSeconds)
		}

		// Initialize output validator from guardrails config
		if guardrailsCfg.OutputSchema != nil && len(guardrailsCfg.OutputSchema) > 0 {
			validator, err := guardrails.NewOutputValidator(guardrailsCfg.OutputSchema)
			if err != nil {
				logger.Warn("Failed to create output validator",
					zap.String("agentspec", spec.GetName()),
					zap.Error(err),
				)
			} else {
				outputValidator = validator
			}
		}

		// Initialize rate limiter from guardrails config
		if guardrailsCfg.RateLimitPerMinute > 0 {
			rateLimiter = guardrails.NewRateLimiter(guardrailsCfg.RateLimitPerMinute, 0)
		}
	}

	return &Keeper{
		spec:           spec,
		logger:         logger,
		store:          store,
		comb:           combStore,
		sessionID:      sessionID,
		cache:          newCombCache(1000),
		hiveCache:      newHiveCache(1000), // Cache up to 1000 SessionID -> HiveName mappings // Cache up to 1000 keys
		metrics:        metricsCollector,
		tokenTracker:   tokenTracker,
		timeoutTracker: timeoutTracker,
		outputValidator: outputValidator,
		rateLimiter:    rateLimiter,
		webhookMgr:     webhookMgr,
		auditLogger:    audit.NewLogger(logger), // Initialize audit logger
		health: &HealthStatus{
			Healthy:   false,
			Timestamp: time.Now(),
		},
	}
}

// Start starts the Keeper and launches the agent process.
func (k *Keeper) Start(ctx context.Context) error {
	k.logger.Info("Keeper starting", zap.String("agentspec", k.spec.GetName()))

	// Launch agent process
	if err := k.launchAgent(ctx); err != nil {
		return fmt.Errorf("failed to launch agent: %w", err)
	}

	// Start health check server
	if err := k.startHealthServer(); err != nil {
		return fmt.Errorf("failed to start health server: %w", err)
	}

	// Start metrics server
	if err := k.startMetricsServer(); err != nil {
		return fmt.Errorf("failed to start metrics server: %w", err)
	}

	// Start health monitoring
	go k.monitorHealth(ctx)

	k.logger.Info("Keeper started")
	return nil
}

// Stop gracefully stops the Keeper and agent process.
func (k *Keeper) Stop(ctx context.Context) error {
	// Shutdown metrics server
	if k.metricsServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = k.metricsServer.Shutdown(shutdownCtx)
	}
	k.logger.Info("Keeper stopping")

	// Stop health server
	if k.server != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = k.server.Shutdown(shutdownCtx)
	}

	// Stop message relay goroutines
	k.mu.Lock()
	if k.relayCancel != nil {
		k.relayCancel() // Cancel context to stop goroutines
	}
	// Unsubscribe from message bus
	if k.busSubscription != nil {
		_ = k.busSubscription.Unsubscribe()
		k.busSubscription = nil
	}
	k.mu.Unlock()

	// Wait for message relay goroutines to finish (with timeout)
	relayDone := make(chan struct{})
	go func() {
		k.relayWg.Wait()
		close(relayDone)
	}()

	select {
	case <-relayDone:
		k.logger.Info("Message relay goroutines stopped")
	case <-ctx.Done():
		k.logger.Warn("Timeout waiting for message relay goroutines to stop")
		return ctx.Err()
	case <-time.After(5 * time.Second):
		k.logger.Warn("Timeout waiting for message relay goroutines to stop")
	}

	// Stop agent process
	if k.process != nil && k.process.Process != nil {
		// Send SIGTERM for graceful shutdown
		if err := k.process.Process.Signal(os.Interrupt); err != nil {
			k.logger.Warn("Failed to send SIGTERM, killing process", zap.Error(err))
			_ = k.process.Process.Kill()
		}

		// Wait for process to exit
		done := make(chan error, 1)
		go func() {
			done <- k.process.Wait()
		}()

		select {
		case <-done:
			k.logger.Info("Agent process stopped")
		case <-ctx.Done():
			_ = k.process.Process.Kill()
			return ctx.Err()
		}
	}

	k.logger.Info("Keeper stopped")
	return nil
}

// launchAgent launches the agent process.
func (k *Keeper) launchAgent(ctx context.Context) error {
	if len(k.spec.Spec.Runtime.Command) == 0 {
		return fmt.Errorf("command is required")
	}

	cmd := exec.CommandContext(ctx, k.spec.Spec.Runtime.Command[0], k.spec.Spec.Runtime.Command[1:]...)

	// Set working directory
	if k.spec.Spec.Runtime.WorkingDir != "" {
		cmd.Dir = k.spec.Spec.Runtime.WorkingDir
	}

	// Set environment variables
	env := os.Environ()
	for _, envVar := range k.spec.Spec.Runtime.Env {
		if envVar.ValueFrom != nil && envVar.ValueFrom.SecretKeyRef != nil {
			// Resolve secret reference
			secretValue, err := k.resolveSecretRef(ctx, k.spec.GetNamespace(), envVar.ValueFrom.SecretKeyRef)
			if err != nil {
				return fmt.Errorf("failed to resolve secret %s for environment variable %s: %w", envVar.ValueFrom.SecretKeyRef.Name, envVar.Name, err)
			}
			env = append(env, fmt.Sprintf("%s=%s", envVar.Name, secretValue))
		} else {
			env = append(env, fmt.Sprintf("%s=%s", envVar.Name, envVar.Value))
		}
	}
	cmd.Env = env

	// Set up I/O based on interface type
	switch k.spec.Spec.Interface.Type {
	case "stdin":
		// Create stdin pipe for writing to agent
		stdinPipe, err := cmd.StdinPipe()
		if err != nil {
			return fmt.Errorf("failed to create stdin pipe: %w", err)
		}
		k.stdinPipe = stdinPipe
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	case "http":
		// For HTTP, agent should start its own server
		// Keeper will connect to it
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	default:
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}

	k.mu.Lock()
	k.process = cmd
	k.mu.Unlock()

	k.logger.Info("Agent process launched",
		zap.Int("pid", cmd.Process.Pid),
		zap.String("interface", k.spec.Spec.Interface.Type),
	)

	return nil
}

// startHealthServer starts the HTTP health check server.
func (k *Keeper) startHealthServer() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", k.handleHealth)
	mux.HandleFunc("/comb/", k.handleComb)
	mux.HandleFunc("/exec", k.handleExec)

	k.server = &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	go func() {
		if err := k.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			k.logger.Error("Health server error", zap.Error(err))
		}
	}()

	return nil
}

// startMetricsServer starts the Prometheus metrics server.
func (k *Keeper) startMetricsServer() error {
	if k.metrics == nil {
		return nil // No metrics collector
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", k.metrics.Handler())

	k.metricsServer = &http.Server{
		Addr:    ":9090",
		Handler: mux,
	}

	go func() {
		if err := k.metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			k.logger.Error("Metrics server error", zap.Error(err))
		}
	}()

	return nil
}

// handleHealth handles health check requests.
func (k *Keeper) handleHealth(w http.ResponseWriter, r *http.Request) {
	k.mu.RLock()
	health := *k.health
	k.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	if health.Healthy {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(health)
}

// handleExec handles exec requests to execute commands in the agent process.
func (k *Keeper) handleExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Command []string `json:"command"`
		Stdin   bool     `json:"stdin,omitempty"`
		TTY     bool     `json:"tty,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if len(req.Command) == 0 {
		http.Error(w, "command is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Execute command in the agent's environment
	cmd := exec.CommandContext(ctx, req.Command[0], req.Command[1:]...)
	
	// Use the same working directory and environment as the agent
	k.mu.RLock()
	if k.process != nil {
		cmd.Dir = k.process.Dir
		cmd.Env = k.process.Env
	}
	k.mu.RUnlock()

	// Set up I/O
	if req.Stdin {
		stdinPipe, err := cmd.StdinPipe()
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create stdin pipe: %v", err), http.StatusInternalServerError)
			return
		}
		go func() {
			defer stdinPipe.Close()
			io.Copy(stdinPipe, r.Body)
		}()
	}

	var stdout, stderr io.ReadCloser
	var err error
	stdout, err = cmd.StdoutPipe()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create stdout pipe: %v", err), http.StatusInternalServerError)
		return
	}

	stderr, err = cmd.StderrPipe()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create stderr pipe: %v", err), http.StatusInternalServerError)
		return
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		http.Error(w, fmt.Sprintf("failed to start command: %v", err), http.StatusInternalServerError)
		return
	}

	// Stream output
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)

	// Stream stdout and stderr
	done := make(chan error, 2)
	go func() {
		_, err := io.Copy(w, stdout)
		done <- err
	}()
	go func() {
		_, err := io.Copy(w, stderr)
		done <- err
	}()

	// Wait for command to complete
	cmdErr := cmd.Wait()
	
	// Wait for output streams to finish
	<-done
	<-done

	// Set exit code in response header
	if cmdErr != nil {
		if exitError, ok := cmdErr.(*exec.ExitError); ok {
			w.Header().Set("X-Exit-Code", fmt.Sprintf("%d", exitError.ExitCode()))
		} else {
			w.Header().Set("X-Exit-Code", "1")
		}
	} else {
		w.Header().Set("X-Exit-Code", "0")
	}
}

// monitorHealth periodically checks the health of the agent process.
func (k *Keeper) monitorHealth(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			k.checkHealth()
		case <-ctx.Done():
			return
		}
	}
}

// checkHealth checks the health of the agent process.
func (k *Keeper) checkHealth() {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.process == nil || k.process.Process == nil {
		k.health.Healthy = false
		k.health.Message = "process not running"
		k.health.Timestamp = time.Now()
		return
	}

	// Check if process is still running
	if err := k.process.Process.Signal(os.Signal(nil)); err != nil {
		k.health.Healthy = false
		k.health.Message = fmt.Sprintf("process check failed: %v", err)
		k.health.Timestamp = time.Now()
		return
	}

	// If agent has HTTP health endpoint, check it
	if k.spec.Spec.Interface.Type == "http" && k.spec.Spec.Interface.HealthPath != "" {
		// TODO: Make HTTP request to agent health endpoint
		k.health.Healthy = true
		k.health.Message = "process running"
	} else {
		// For stdin/stdout, just check process is alive
		k.health.Healthy = true
		k.health.Message = "process running"
	}

	k.health.Timestamp = time.Now()

	// Check for timeouts
	if k.timeoutTracker != nil {
		timedOut := k.timeoutTracker.CheckTimeouts()
		if len(timedOut) > 0 {
			k.health.Healthy = false
			k.health.Message = fmt.Sprintf("%d request(s) timed out", len(timedOut))
			// Audit log and fire webhook for timeout violations
			for _, msgID := range timedOut {
				// Audit log guardrail violation
				if k.auditLogger != nil && k.spec != nil {
					hiveName, _ := k.getHiveNameFromSession(context.Background(), k.sessionID)
					timeoutErr := fmt.Errorf("request timed out")
					k.auditLogger.LogGuardrailViolation(
						"timeout",
						k.spec.GetNamespace(),
						hiveName,
						k.spec.GetName(),
						k.agentID,
						k.sessionID,
						msgID,
						timeoutErr.Error(),
						map[string]interface{}{
							"timeoutSeconds": k.timeoutTracker.GetTimeoutSeconds(),
						},
					)
				}
				// Fire webhook for timeout violations
				k.fireWebhook(context.Background(), "timeout", msgID, fmt.Errorf("request timed out"))
			}
		}
	}
}

// getHiveNameFromSession looks up the Hive name for a SessionID.
// Uses cache first, then queries store on cache miss.
// Returns empty string and no error if session doesn't belong to a Hive.
func (k *Keeper) getHiveNameFromSession(ctx context.Context, sessionID string) (string, error) {
	if sessionID == "" {
		return "", nil // No session, no Hive
	}

	// Check cache first
	if hiveName, exists := k.hiveCache.get(sessionID); exists {
		if hiveName == "" {
			return "", nil // Cached as "no Hive"
		}
		return hiveName, nil
	}

	// Cache miss: query store
	if k.store == nil {
		// No store available, return empty (no Hive)
		return "", nil
	}

	namespace := k.spec.GetNamespace()
	
	// Query Session from store
	resource, err := k.store.Get(ctx, "Session", sessionID, namespace)
	if err != nil {
		// Session not found or error - cache empty result to avoid repeated lookups
		k.hiveCache.set(sessionID, "")
		return "", nil
	}

	session, ok := resource.(*apiary.Session)
	if !ok {
		// Wrong type - cache empty result
		k.hiveCache.set(sessionID, "")
		return "", nil
	}

	// Extract Hive name from annotations
	hiveName := ""
	if session.GetAnnotations() != nil {
		hiveName = session.GetAnnotations()["apiary.io/hive-name"]
	}

	// Cache the result (even if empty, to avoid repeated lookups)
	k.hiveCache.set(sessionID, hiveName)

	return hiveName, nil
}

// getHiveWebhookConfig gets the webhook configuration for a Hive.
func (k *Keeper) getHiveWebhookConfig(ctx context.Context, hiveName string) (string, string, error) {
	if k.store == nil || hiveName == "" {
		return "", "", nil // No store or no Hive
	}

	namespace := k.spec.GetNamespace()
	resource, err := k.store.Get(ctx, "Hive", hiveName, namespace)
	if err != nil {
		return "", "", nil // Hive not found, no webhook
	}

	hive, ok := resource.(*apiary.Hive)
	if !ok {
		return "", "", nil // Not a Hive
	}

	webhookCfg := hive.Spec.Webhook
	return webhookCfg.URL, webhookCfg.AuthHeader, nil
}

// fireWebhook fires a webhook event for a guardrail violation.
func (k *Keeper) fireWebhook(ctx context.Context, eventType, messageID string, err error) {
	if k.webhookMgr == nil {
		return // No webhook manager
	}

	// Get Hive name from session
	hiveName, _ := k.getHiveNameFromSession(ctx, k.sessionID)
	if hiveName == "" {
		return // No Hive associated
	}

	// Get webhook config from Hive
	webhookURL, authHeader, _ := k.getHiveWebhookConfig(ctx, hiveName)
	if webhookURL == "" {
		return // No webhook configured
	}

	// Create webhook event
	event := &webhook.Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Namespace: k.spec.GetNamespace(),
		HiveName:  hiveName,
		AgentSpec: k.spec.GetName(),
		DroneID:   k.agentID,
		SessionID: k.sessionID,
		MessageID: messageID,
		Error:     err.Error(),
		Details:   make(map[string]interface{}),
	}

	// Add event-specific details
	switch eventType {
	case "rate_limit":
		if k.rateLimiter != nil {
			event.Details["requestsPerMinute"] = k.rateLimiter.RequestsPerMinute()
		}
	case "token_budget":
		if k.tokenTracker != nil {
			// Add token usage details if available
			event.Details["budgetExceeded"] = true
		}
	case "timeout":
		if k.timeoutTracker != nil {
			event.Details["activeRequests"] = k.timeoutTracker.GetActiveRequestCount()
		}
	case "output_validation":
		event.Details["validationFailed"] = true
	}

	// Send webhook (fire and forget)
	_ = k.webhookMgr.SendEvent(ctx, webhookURL, authHeader, event)
}

// SendMessage sends a message to the agent (for stdin interface).
func (k *Keeper) SendMessage(msg *waggle.Message) error {
	start := time.Now()
	
	if k.spec.Spec.Interface.Type != "stdin" {
		return fmt.Errorf("SendMessage only supported for stdin interface")
	}

	k.mu.RLock()
	stdinPipe := k.stdinPipe
	k.mu.RUnlock()

	if stdinPipe == nil {
		return fmt.Errorf("stdin pipe not available")
	}

	data, err := waggle.EncodeMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to encode message: %w", err)
	}

	// Write to stdin pipe
	_, err = stdinPipe.Write(append(data, '\n'))
	
	// Record metrics
	if k.metrics != nil && err == nil {
		latency := time.Since(start)
		k.metrics.RecordLatency(
			k.spec.GetName(),
			k.spec.GetNamespace(),
			k.agentID,
			latency,
		)
		k.metrics.RecordRequest(
			k.spec.GetName(),
			k.spec.GetNamespace(),
			k.agentID,
		)
	}
	
	return err
}

// ReceiveMessage receives a message from the agent (for stdout interface).
func (k *Keeper) ReceiveMessage() (*waggle.Message, error) {
	// This would typically read from stdout
	// For now, return error as this requires more complex implementation
	return nil, fmt.Errorf("ReceiveMessage not yet implemented")
}

// handleComb handles Comb proxy requests.
func (k *Keeper) handleComb(w http.ResponseWriter, r *http.Request) {
	// Extract key from path: /comb/{key}
	key := r.URL.Path[len("/comb/"):]
	if key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		// Check cache first
		if cached, ok := k.cache.get(key); ok {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"value": cached})
			return
		}

		// Get from Comb using namespace-prefixed sessionID
		namespace := k.spec.GetNamespace()
		namespacedSessionID := fmt.Sprintf("%s:%s", namespace, k.sessionID)
		value, err := k.comb.Get(ctx, namespacedSessionID, key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		// Cache the value
		k.cache.set(key, value, 0)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"value": value})

	case http.MethodPost, http.MethodPut:
		var req struct {
			Value string        `json:"value"`
			TTL   time.Duration `json:"ttl,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Set in Comb using namespace-prefixed sessionID
		namespace := k.spec.GetNamespace()
		namespacedSessionID := fmt.Sprintf("%s:%s", namespace, k.sessionID)
		if err := k.comb.Set(ctx, namespacedSessionID, key, req.Value, req.TTL); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Invalidate cache
		k.cache.invalidate(key)

		w.WriteHeader(http.StatusOK)

	case http.MethodDelete:
		// Delete from Comb using namespace-prefixed sessionID
		namespace := k.spec.GetNamespace()
		namespacedSessionID := fmt.Sprintf("%s:%s", namespace, k.sessionID)
		if err := k.comb.Delete(ctx, namespacedSessionID, key); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		// Invalidate cache
		k.cache.invalidate(key)

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// resolveSecretRef resolves a secret reference and returns the secret value.
func (k *Keeper) resolveSecretRef(ctx context.Context, namespace string, ref *apiary.SecretKeySelector) (string, error) {
	if k.store == nil {
		return "", fmt.Errorf("store not available for secret resolution")
	}

	// Get secret from store
	resource, err := k.store.Get(ctx, "Secret", ref.Name, namespace)
	if err != nil {
		return "", fmt.Errorf("failed to get secret %s: %w", ref.Name, err)
	}

	secret, ok := resource.(*apiary.Secret)
	if !ok {
		return "", fmt.Errorf("resource %s is not a Secret", ref.Name)
	}

	// Verify secret is in the same namespace
	if secret.GetNamespace() != namespace {
		return "", fmt.Errorf("secret %s is in namespace %s, but agent is in namespace %s", ref.Name, secret.GetNamespace(), namespace)
	}

	// Get the key value
	value, exists := secret.Data[ref.Key]
	if !exists {
		return "", fmt.Errorf("key %s not found in secret %s", ref.Key, ref.Name)
	}

	return string(value), nil
}
