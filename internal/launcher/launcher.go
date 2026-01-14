// Package launcher provides agent process launching functionality.
package launcher

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Launcher implements the apiary.AgentLauncher interface.
type Launcher struct {
	logger    *zap.Logger
	store     apiary.ResourceStore
	processes map[string]*apiary.AgentProcess
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
}

var _ apiary.AgentLauncher = (*Launcher)(nil)

// NewLauncher creates a new agent launcher.
func NewLauncher(logger *zap.Logger, store apiary.ResourceStore) *Launcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &Launcher{
		logger:    logger,
		store:     store,
		processes: make(map[string]*apiary.AgentProcess),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Shutdown gracefully shuts down the launcher, cancelling all monitor goroutines.
func (l *Launcher) Shutdown(ctx context.Context) error {
	l.cancel()
	// Wait for all processes to be cleaned up
	// In a real implementation, we might want to wait for monitor goroutines
	return nil
}

// Launch starts an agent process.
func (l *Launcher) Launch(ctx context.Context, spec *apiary.AgentSpec) (*apiary.AgentProcess, error) {
	if len(spec.Spec.Runtime.Command) == 0 {
		return nil, fmt.Errorf("command is required")
	}

	// Create command
	cmd := exec.CommandContext(ctx, spec.Spec.Runtime.Command[0], spec.Spec.Runtime.Command[1:]...)

	// Set working directory
	if spec.Spec.Runtime.WorkingDir != "" {
		cmd.Dir = spec.Spec.Runtime.WorkingDir
	}

	// Set environment variables
	env := os.Environ()
	for _, envVar := range spec.Spec.Runtime.Env {
		if envVar.ValueFrom != nil && envVar.ValueFrom.SecretKeyRef != nil {
			// Resolve secret reference
			secretValue, err := l.resolveSecretRef(ctx, spec.GetNamespace(), envVar.ValueFrom.SecretKeyRef)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve secret %s for environment variable %s: %w", envVar.ValueFrom.SecretKeyRef.Name, envVar.Name, err)
			}
			env = append(env, fmt.Sprintf("%s=%s", envVar.Name, secretValue))
		} else {
			env = append(env, fmt.Sprintf("%s=%s", envVar.Name, envVar.Value))
		}
	}
	cmd.Env = env

	// Set up stdout/stderr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start the process
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start process: %w", err)
	}

	processID := uuid.New().String()
	now := time.Now()

	process := &apiary.AgentProcess{
		ID:        processID,
		PID:       cmd.Process.Pid,
		Status:    apiary.ProcessStatusRunning,
		StartedAt: now,
		Spec:      spec,
	}

	l.mu.Lock()
	l.processes[processID] = process
	l.mu.Unlock()

	// Monitor process in background
	go l.monitorProcess(l.ctx, processID, cmd)

	l.logger.Info("Launched agent process",
		zap.String("id", processID),
		zap.Int("pid", process.PID),
		zap.String("agentspec", spec.GetName()),
	)

	return process, nil
}

// resolveSecretRef resolves a secret reference and returns the secret value.
func (l *Launcher) resolveSecretRef(ctx context.Context, namespace string, ref *apiary.SecretKeySelector) (string, error) {
	if l.store == nil {
		return "", fmt.Errorf("store not available for secret resolution")
	}

	// Get secret from store
	resource, err := l.store.Get(ctx, "Secret", ref.Name, namespace)
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

// Stop gracefully stops an agent process.
func (l *Launcher) Stop(ctx context.Context, processID string) error {
	l.mu.Lock()
	process, exists := l.processes[processID]
	if !exists {
		l.mu.Unlock()
		return fmt.Errorf("process not found: %s", processID)
	}
	// Remove from map immediately to prevent concurrent access issues
	// and ensure monitorProcess doesn't try to update it
	delete(l.processes, processID)
	l.mu.Unlock()

	// Find the process
	proc, err := os.FindProcess(process.PID)
	if err != nil {
		// Process may already be dead, which is fine
		// Already removed from map, so we're done
		l.logger.Warn("Failed to find process (may already be dead)",
			zap.String("id", processID),
			zap.Int("pid", process.PID),
			zap.Error(err),
		)
		return nil
	}

	// Check if process is still alive by sending signal 0 (no-op signal)
	// This doesn't actually send a signal but checks if the process exists
	if err := proc.Signal(os.Signal(nil)); err != nil {
		// Process is already dead - cleanup is complete
		l.logger.Info("Process already dead",
			zap.String("id", processID),
			zap.Int("pid", process.PID),
		)
		return nil
	}

	// Process is alive - send SIGTERM for graceful shutdown
	if err := proc.Signal(os.Interrupt); err != nil {
		// If SIGTERM fails, try SIGKILL
		l.logger.Warn("SIGTERM failed, trying SIGKILL",
			zap.String("id", processID),
			zap.Error(err),
		)
		if killErr := proc.Kill(); killErr != nil {
			// Even if kill fails, process may have exited
			// Log the error but don't fail - process is removed from map
			l.logger.Error("Failed to kill process (may have already exited)",
				zap.String("id", processID),
				zap.Error(killErr),
			)
			// Don't return error - process is removed from tracking
		}
	} else {
		// Wait a bit for graceful shutdown, then force kill if needed
		done := make(chan error, 1)
		go func() {
			// Wait for process to exit
			_, waitErr := proc.Wait()
			done <- waitErr
		}()

		select {
		case <-done:
			// Process exited
		case <-ctx.Done():
			// Context cancelled, force kill
			_ = proc.Kill()
		case <-time.After(5 * time.Second):
			// Timeout, force kill
			l.logger.Warn("Process did not exit gracefully, forcing kill",
				zap.String("id", processID),
			)
			_ = proc.Kill()
		}
	}

	l.logger.Info("Stopped agent process", zap.String("id", processID))
	return nil
}

// GetProcess returns information about a running process.
func (l *Launcher) GetProcess(ctx context.Context, processID string) (*apiary.AgentProcess, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	process, exists := l.processes[processID]
	if !exists {
		return nil, fmt.Errorf("process not found: %s", processID)
	}

	return process, nil
}

// monitorProcess monitors a process and updates its status.
func (l *Launcher) monitorProcess(ctx context.Context, processID string, cmd *exec.Cmd) {
	// Use a channel to wait for process exit or context cancellation
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var err error
	select {
	case err = <-done:
		// Process exited normally
	case <-ctx.Done():
		// Context cancelled - launcher is shutting down
		// Don't update status, just return
		l.logger.Debug("Process monitor cancelled",
			zap.String("id", processID),
		)
		return
	}

	l.mu.Lock()
	process, exists := l.processes[processID]
	if exists {
		if err != nil {
			process.Status = apiary.ProcessStatusFailed
		} else {
			process.Status = apiary.ProcessStatusStopped
		}
	}
	l.mu.Unlock()

	if err != nil {
		l.logger.Warn("Process exited with error",
			zap.String("id", processID),
			zap.Error(err),
		)
	} else {
		l.logger.Info("Process exited",
			zap.String("id", processID),
		)
	}
}
