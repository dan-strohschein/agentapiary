package cmd

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec",
	Short: "Execute a command in a Drone's Keeper",
	Long: `Execute a command in a Drone's Keeper container.

The exec command allows you to execute commands inside a Drone's Keeper,
which runs in the same environment as the agent process.

Examples:
  # Execute a command
  apiaryctl exec drone/my-drone -n default -- /bin/sh

  # Execute with arguments
  apiaryctl exec drone/my-drone -n default -- ls -la

  # Interactive shell
  apiaryctl exec drone/my-drone -n default -it -- /bin/bash`,
	RunE: runExec,
}

var (
	execInteractive bool
	execTTY         bool
)

func init() {
	rootCmd.AddCommand(execCmd)
	execCmd.Flags().BoolVarP(&execInteractive, "stdin", "i", false, "Pass stdin to the container")
	execCmd.Flags().BoolVarP(&execTTY, "tty", "t", false, "Allocate a TTY")
}

func runExec(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("drone name and command are required (format: drone/<name> -- <command>...)")
	}

	// Find the -- separator
	separatorIndex := -1
	for i, arg := range args {
		if arg == "--" {
			separatorIndex = i
			break
		}
	}

	if separatorIndex == -1 {
		return fmt.Errorf("command separator '--' is required")
	}

	if separatorIndex == 0 {
		return fmt.Errorf("drone name is required before '--'")
	}

	if separatorIndex == len(args)-1 {
		return fmt.Errorf("command is required after '--'")
	}

	// Parse drone name
	droneName := args[0]
	if strings.HasPrefix(droneName, "drone/") {
		droneName = strings.TrimPrefix(droneName, "drone/")
	}

	// Parse command
	command := args[separatorIndex+1:]

	client := NewAPIClient()
	namespace := GetNamespace()

	// Build exec API path
	execPath := fmt.Sprintf("/api/v1/cells/%s/drones/%s/exec", namespace, droneName)

	// Prepare request body
	reqBody := map[string]interface{}{
		"command": command,
		"stdin":   execInteractive,
		"tty":     execTTY,
	}

	// For interactive mode, we need to handle stdin differently
	// For now, use the standard POST method
	resp, err := client.Post(execPath, reqBody)
	if err != nil {
		return fmt.Errorf("failed to execute command: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("exec failed (status %d): %s", resp.StatusCode, string(body))
	}

	// Handle signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Stream output
	done := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			cmd.Println(scanner.Text())
		}
		done <- scanner.Err()
	}()

	// Wait for completion or signal
	select {
	case err := <-done:
		if err != nil && err != io.EOF {
			return fmt.Errorf("failed to read output: %w", err)
		}
	case <-sigChan:
		cmd.Println("\nInterrupted")
		return nil
	}

	// Get exit code from header
	if exitCode := resp.Header.Get("X-Exit-Code"); exitCode != "" {
		if exitCode != "0" {
			return fmt.Errorf("command exited with code %s", exitCode)
		}
	}

	return nil
}
