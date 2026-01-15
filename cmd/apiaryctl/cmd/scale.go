package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var scaleCmd = &cobra.Command{
	Use:   "scale",
	Short: "Scale a resource",
	Long: `Scale a resource (supports AgentSpec and Hive scaling).

Examples:
  # Scale an AgentSpec to 3 replicas
  apiaryctl scale agentspec my-agent --replicas=3 -n default

  # Scale a Hive to 5 replicas (all stages)
  apiaryctl scale hive my-hive --replicas=5 -n default

  # Scale a specific Hive stage
  apiaryctl scale hive my-hive --replicas=3 --stage=stage1 -n default

  # Scale to 0 (stop all Drones)
  apiaryctl scale agentspec my-agent --replicas=0 -n default`,
	RunE: runScale,
}

var (
	scaleReplicas int
	scaleStage    string
)

func init() {
	rootCmd.AddCommand(scaleCmd)
	scaleCmd.Flags().IntVar(&scaleReplicas, "replicas", 0, "Number of replicas")
	scaleCmd.Flags().StringVar(&scaleStage, "stage", "", "Stage name (for Hive scaling only)")
	scaleCmd.MarkFlagRequired("replicas")
}

func runScale(cmd *cobra.Command, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("resource type and name are required")
	}

	resourceType := strings.ToLower(args[0])
	resourceName := args[1]

	// Normalize resource type
	resourceType = normalizeResourceType(resourceType)

	if resourceType != "agentspec" && resourceType != "hive" {
		return fmt.Errorf("unsupported resource type for scaling: %s (supported: agentspec, hive)", resourceType)
	}

	client := NewAPIClient()
	namespace := GetNamespace()

	// Build scale API path
	var scalePath string
	if resourceType == "agentspec" {
		scalePath = fmt.Sprintf("/api/v1/cells/%s/agentspecs/%s/scale", namespace, resourceName)
	} else {
		scalePath = fmt.Sprintf("/api/v1/cells/%s/hives/%s/scale", namespace, resourceName)
	}

	// Prepare request body
	reqBody := map[string]interface{}{
		"replicas": scaleReplicas,
	}

	if resourceType == "hive" && scaleStage != "" {
		reqBody["stage"] = scaleStage
	}

	// Send scale request
	resp, err := client.Put(scalePath, reqBody)
	if err != nil {
		return fmt.Errorf("failed to scale resource: %w", err)
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
		jsonBytes, _ := json.MarshalIndent(result, "", "  ")
		cmd.Println(string(jsonBytes))
	}

	return nil
}
