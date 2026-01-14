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
	"github.com/agentapiary/apiary/internal/launcher"
	"github.com/agentapiary/apiary/internal/queen"
	"github.com/agentapiary/apiary/internal/scheduler"
	"github.com/agentapiary/apiary/internal/store/badger"
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
		Port:   *port,
		Store:  store,
		RBAC:   rbac,
		Logger: logger,
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
