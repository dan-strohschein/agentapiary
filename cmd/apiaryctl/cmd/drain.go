package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var drainCmd = &cobra.Command{
	Use:   "drain",
	Short: "Drain a resource (gracefully stop and remove)",
	Long: `Drain a resource, gracefully stopping and removing it.

Draining marks a resource for graceful shutdown, waits for in-flight
requests to complete, removes it from rotation, and then terminates it.

Examples:
  # Drain a Drone (gracefully stop)
  apiaryctl drain drone/my-drone -n default

  # Force drain (immediate termination)
  apiaryctl drain drone/my-drone --force -n default

  # Drain with custom timeout
  apiaryctl drain drone/my-drone --timeout=60s -n default

  # Drain all Drones for an AgentSpec
  apiaryctl drain agentspec/my-agent -n default`,
	RunE: runDrain,
}

var (
	drainForce   bool
	drainTimeout string
)

func init() {
	rootCmd.AddCommand(drainCmd)
	drainCmd.Flags().BoolVar(&drainForce, "force", false, "Force drain without graceful shutdown")
	drainCmd.Flags().StringVar(&drainTimeout, "timeout", "30s", "Timeout for graceful shutdown")
}

func runDrain(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("resource name is required (format: drone/<name> or agentspec/<name>)")
	}

	// Parse resource name
	resourceName := args[0]
	var resourceType, name string

	if strings.HasPrefix(resourceName, "drone/") {
		resourceType = "drone"
		name = strings.TrimPrefix(resourceName, "drone/")
	} else if strings.HasPrefix(resourceName, "agentspec/") {
		resourceType = "agentspec"
		name = strings.TrimPrefix(resourceName, "agentspec/")
	} else {
		// Try to infer from format
		if strings.Contains(resourceName, "/") {
			parts := strings.Split(resourceName, "/")
			if len(parts) == 2 {
				resourceType = parts[0]
				name = parts[1]
			} else {
				return fmt.Errorf("invalid resource format: %s (expected: drone/<name> or agentspec/<name>)", resourceName)
			}
		} else {
			return fmt.Errorf("resource type required: %s (expected: drone/<name> or agentspec/<name>)", resourceName)
		}
	}

	if resourceType != "drone" && resourceType != "agentspec" {
		return fmt.Errorf("unsupported resource type for draining: %s (supported: drone, agentspec)", resourceType)
	}

	// Validate timeout format
	if drainTimeout != "" {
		if _, err := time.ParseDuration(drainTimeout); err != nil {
			return fmt.Errorf("invalid timeout format: %v", err)
		}
	}

	client := NewAPIClient()
	namespace := GetNamespace()

	// Build drain API path
	var drainPath string
	if resourceType == "drone" {
		drainPath = fmt.Sprintf("/api/v1/cells/%s/drones/%s/drain", namespace, name)
	} else {
		drainPath = fmt.Sprintf("/api/v1/cells/%s/agentspecs/%s/drain", namespace, name)
	}

	// Prepare request body
	reqBody := map[string]interface{}{
		"force": drainForce,
	}
	if drainTimeout != "" {
		reqBody["timeout"] = drainTimeout
	}

	// Send drain request
	cmd.Printf("Draining %s/%s...\n", resourceType, name)
	resp, err := client.Post(drainPath, reqBody)
	if err != nil {
		return fmt.Errorf("failed to drain resource: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	var result map[string]interface{}
	if err := client.HandleResponse(resp, &result); err != nil {
		return err
	}

	// Print success message
	if message, ok := result["message"].(string); ok {
		cmd.Println(message)
	} else {
		// Fallback: print JSON
		cmd.Printf("Drain completed: %+v\n", result)
	}

	// Print additional info if available
	if drainedCount, ok := result["drainedCount"].(float64); ok {
		cmd.Printf("Drained %d resource(s)\n", int(drainedCount))
	}

	if errors, ok := result["errors"].([]interface{}); ok && len(errors) > 0 {
		cmd.Printf("Errors encountered:\n")
		for _, errMsg := range errors {
			cmd.Printf("  - %v\n", errMsg)
		}
	}

	return nil
}
