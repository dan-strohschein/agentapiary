package main

import (
	"flag"
	"fmt"
	"os"

	"go.uber.org/zap"
)

func main() {
	var (
		agentSpecPath = flag.String("agent-spec", "", "Path to agent spec file")
	)
	flag.Parse()

	if *agentSpecPath == "" {
		fmt.Fprintf(os.Stderr, "agent-spec is required\n")
		os.Exit(1)
	}

	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Keeper starting", zap.String("agent-spec", *agentSpecPath))

	// TODO: Implement Keeper logic
	logger.Info("Keeper implementation pending")
}
