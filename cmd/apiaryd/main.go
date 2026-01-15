package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/agentapiary/apiary/internal/api"
	"github.com/agentapiary/apiary/internal/auth"
	"github.com/agentapiary/apiary/internal/dlq"
	"github.com/agentapiary/apiary/internal/launcher"
	"github.com/agentapiary/apiary/internal/metrics"
	"github.com/agentapiary/apiary/internal/queen"
	"github.com/agentapiary/apiary/internal/scheduler"
	"github.com/agentapiary/apiary/internal/session"
	"github.com/agentapiary/apiary/internal/store/badger"
	"github.com/agentapiary/apiary/internal/webhook"
	"go.uber.org/zap"
)

func main() {
	var (
		dataDir = flag.String("data-dir", "/var/apiary", "Data directory for BadgerDB")
		port    = flag.Int("port", 8080, "API server port")
	)
	flag.Parse()

	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// Initialize store
	store, err := badger.NewStore(*dataDir)
	if err != nil {
		logger.Fatal("Failed to create store", zap.Error(err))
	}
	defer store.Close()

	// Initialize scheduler
	sched := scheduler.NewScheduler(logger)

	// Initialize launcher
	launch := launcher.NewLauncher(logger, store)

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector(logger)

	// Initialize DLQ Manager
	dlqMgr := dlq.NewManager(logger)

	// Initialize webhook manager
	// Note: webhookMgr should be passed to Keeper when Keepers are created
	// For now, it's initialized here but will need to be integrated into the Keeper creation flow
	webhookMgr := webhook.NewManager(webhook.Config{
		Timeout:   10 * time.Second,
		MaxRetries: 3,
		RetryDelay: 1 * time.Second,
		Logger:    logger,
	}, metricsCollector)
	_ = webhookMgr // TODO: Pass to Keeper when Keepers are created

	// Initialize session manager
	sessionMgr := session.NewManager(session.Config{
		Store:  store,
		Logger: logger,
	})
	if err := sessionMgr.Start(context.Background()); err != nil {
		logger.Fatal("Failed to start session manager", zap.Error(err))
	}

	// Initialize Queen
	q := queen.NewQueen(queen.Config{
		Store:     store,
		Scheduler:  sched,
		Launcher:   launch,
		Logger:     logger,
	})

	// Initialize RBAC
	rbac := auth.NewRBAC()

	// Initialize API server
	apiServer, err := api.NewServer(api.Config{
		Port:       *port,
		Store:      store,
		SessionMgr: sessionMgr,
		Metrics:    metricsCollector,
		RBAC:       rbac,
		DLQManager: dlqMgr,
		Logger:     logger,
	})
	if err != nil {
		logger.Fatal("Failed to create API server", zap.Error(err))
	}

	// Start Queen
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := q.Start(ctx); err != nil {
		logger.Fatal("Failed to start Queen", zap.Error(err))
	}

	// Start API server
	addr := fmt.Sprintf(":%d", *port)
	go func() {
		logger.Info("Starting API server", zap.String("addr", addr))
		if err := apiServer.Start(addr); err != nil {
			logger.Fatal("API server failed", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Stop Queen
	if err := q.Stop(shutdownCtx); err != nil {
		logger.Error("Error stopping Queen", zap.Error(err))
	}

	// Stop API server
	if err := apiServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("Error shutting down API server", zap.Error(err))
	}
}
