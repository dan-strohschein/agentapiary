// Package queen provides leader election for Queen instances.
package queen

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"
)

// LeaderElector provides leader election functionality using etcd.
// NOTE: This is a stub implementation. See build-docs/remaining.md for details on completing it.
type LeaderElector interface {
	// Start starts the leader election process.
	Start(ctx context.Context) error
	// Stop stops the leader election process.
	Stop(ctx context.Context) error
	// IsLeader returns true if this instance is the current leader.
	IsLeader() bool
	// OnLeaderElected is called when this instance becomes the leader.
	OnLeaderElected(func(context.Context) error)
	// OnLeaderLost is called when this instance loses leadership.
	OnLeaderLost(func(context.Context) error)
}

// leaderElector implements LeaderElector using etcd.
// NOTE: This is a stub implementation. The etcd client library (go.etcd.io/etcd/client/v3) is not yet added.
// See build-docs/remaining.md for implementation details.
type leaderElector struct {
	endpoints []string
	key       string
	leaseTTL  int
	logger    *zap.Logger
	mu        sync.RWMutex
	isLeader  bool
	stopCh    chan struct{}
	wg        sync.WaitGroup
	onElected func(context.Context) error
	onLost    func(context.Context) error
	// etcdClient *clientv3.Client  // TODO: Add etcd client when dependency is added
	// session    *concurrency.Session  // TODO: Add etcd session when dependency is added
	// election   *concurrency.Election  // TODO: Add etcd election when dependency is added
}

// NewLeaderElector creates a new leader elector.
// NOTE: This is a stub implementation. See build-docs/remaining.md for details on completing it.
func NewLeaderElector(endpoints []string, key string, leaseTTL int, logger *zap.Logger) LeaderElector {
	if len(endpoints) == 0 {
		// No etcd endpoints - return a no-op elector (always leader, single-instance mode)
		return &noOpLeaderElector{isLeader: true}
	}

	defaultKey := "/apiary/leader"
	if key == "" {
		key = defaultKey
	}

	defaultTTL := 30
	if leaseTTL == 0 {
		leaseTTL = defaultTTL
	}

	return &leaderElector{
		endpoints: endpoints,
		key:       key,
		leaseTTL:  leaseTTL,
		logger:    logger,
		stopCh:    make(chan struct{}),
	}
}

// Start starts the leader election process.
// NOTE: This is a stub implementation. See build-docs/remaining.md for implementation details.
func (l *leaderElector) Start(ctx context.Context) error {
	l.logger.Info("Leader election starting (stub implementation - not functional)",
		zap.Strings("endpoints", l.endpoints),
		zap.String("key", l.key),
		zap.Int("leaseTTL", l.leaseTTL),
	)

	// TODO: Implement etcd leader election
	// 1. Create etcd client: clientv3.New(clientv3.Config{Endpoints: l.endpoints})
	// 2. Create session with TTL: concurrency.NewSession(client, concurrency.WithTTL(l.leaseTTL))
	// 3. Create election: concurrency.NewElection(session, l.key)
	// 4. Start election loop: election.Campaign(ctx, instanceID)
	// 5. Monitor leadership status and call onElected/onLost callbacks
	// See build-docs/remaining.md for complete implementation details

	return fmt.Errorf("etcd leader election not yet implemented - see build-docs/remaining.md")
}

// Stop stops the leader election process.
func (l *leaderElector) Stop(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.stopCh == nil {
		return nil
	}

	close(l.stopCh)
	l.stopCh = nil

	// Wait for goroutines to finish
	done := make(chan struct{})
	go func() {
		l.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		l.logger.Info("Leader election stopped")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsLeader returns true if this instance is the current leader.
func (l *leaderElector) IsLeader() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.isLeader
}

// OnLeaderElected sets the callback for when this instance becomes the leader.
func (l *leaderElector) OnLeaderElected(callback func(context.Context) error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onElected = callback
}

// OnLeaderLost sets the callback for when this instance loses leadership.
func (l *leaderElector) OnLeaderLost(callback func(context.Context) error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onLost = callback
}

// noOpLeaderElector is a no-op implementation that always considers itself the leader.
// Used when no etcd endpoints are configured (single-instance mode).
type noOpLeaderElector struct {
	isLeader bool
	mu       sync.RWMutex
}

func (n *noOpLeaderElector) Start(ctx context.Context) error {
	return nil
}

func (n *noOpLeaderElector) Stop(ctx context.Context) error {
	return nil
}

func (n *noOpLeaderElector) IsLeader() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.isLeader
}

func (n *noOpLeaderElector) OnLeaderElected(callback func(context.Context) error) {
	// No-op - single instance is always leader
}

func (n *noOpLeaderElector) OnLeaderLost(callback func(context.Context) error) {
	// No-op - single instance never loses leadership
}
