package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var describeCmd = &cobra.Command{
	Use:   "describe",
	Short: "Show details of a specific resource",
	Long: `Show detailed information about a specific resource.

Examples:
  # Describe a Hive
  apiaryctl describe hive my-hive -n default

  # Describe an AgentSpec
  apiaryctl describe agentspec my-agent -n default

  # Describe a Drone
  apiaryctl describe drone my-drone -n default

  # Describe a Cell
  apiaryctl describe cell my-cell`,
	RunE: runDescribe,
}

func init() {
	rootCmd.AddCommand(describeCmd)
}

func runDescribe(cmd *cobra.Command, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("resource type and name are required")
	}

	client := NewAPIClient()
	namespace := GetNamespace()

	// Parse resource type and name
	resourceType := strings.ToLower(args[0])
	resourceName := args[1]

	// Normalize resource type
	resourceType = normalizeResourceType(resourceType)

	// Build API path
	apiPath, err := getGetPath(resourceType, resourceName, namespace)
	if err != nil {
		return err
	}

	// Get resource
	resp, err := client.Get(apiPath)
	if err != nil {
		return fmt.Errorf("failed to get resource: %w", err)
	}

	var resource map[string]interface{}
	if err := client.HandleResponse(resp, &resource); err != nil {
		return err
	}

	// Print formatted description
	return printResourceDescription(os.Stdout, resource, resourceType)
}

// printResourceDescription prints a human-readable description of a resource.
func printResourceDescription(w *os.File, resource map[string]interface{}, resourceType string) error {
	metadata, _ := resource["metadata"].(map[string]interface{})
	spec, _ := resource["spec"].(map[string]interface{})
	status, _ := resource["status"].(map[string]interface{})

	// Print Name and basic info
	name := getString(metadata, "name")
	namespace := getString(metadata, "namespace")
	uid := getString(metadata, "uid")
	created := getString(metadata, "createdAt")
	updated := getString(metadata, "updatedAt")

	fmt.Fprintf(w, "Name:         %s\n", name)
	if namespace != "" {
		fmt.Fprintf(w, "Namespace:    %s\n", namespace)
	}
	fmt.Fprintf(w, "UID:          %s\n", uid)
	fmt.Fprintf(w, "Created:      %s\n", formatTime(created))
	if updated != "" {
		fmt.Fprintf(w, "Updated:      %s\n", formatTime(updated))
	}

	// Print Labels
	if labels, ok := metadata["labels"].(map[string]interface{}); ok && len(labels) > 0 {
		fmt.Fprintln(w, "\nLabels:")
		for k, v := range labels {
			fmt.Fprintf(w, "  %s=%s\n", k, v)
		}
	}

	// Print Annotations
	if annotations, ok := metadata["annotations"].(map[string]interface{}); ok && len(annotations) > 0 {
		fmt.Fprintln(w, "\nAnnotations:")
		for k, v := range annotations {
			fmt.Fprintf(w, "  %s=%s\n", k, v)
		}
	}

	// Print Status (if available)
	if status != nil && len(status) > 0 {
		fmt.Fprintln(w, "\nStatus:")
		printStatus(w, status, resourceType)
	}

	// Print Spec (resource-specific)
	if spec != nil && len(spec) > 0 {
		fmt.Fprintln(w, "\nSpec:")
		printSpec(w, spec, resourceType)
	}

	return nil
}

// printStatus prints status information based on resource type.
func printStatus(w *os.File, status map[string]interface{}, resourceType string) {
	switch resourceType {
	case "drone":
		if phase, ok := status["phase"].(string); ok {
			fmt.Fprintf(w, "  Phase:       %s\n", phase)
		}
		if message, ok := status["message"].(string); ok && message != "" {
			fmt.Fprintf(w, "  Message:     %s\n", message)
		}
		if startedAt, ok := status["startedAt"].(string); ok && startedAt != "" {
			fmt.Fprintf(w, "  Started:     %s\n", formatTime(startedAt))
		}
		if keeperAddr, ok := status["keeperAddr"].(string); ok && keeperAddr != "" {
			fmt.Fprintf(w, "  Keeper Addr: %s\n", keeperAddr)
		}
		if health, ok := status["health"].(map[string]interface{}); ok {
			if healthy, ok := health["healthy"].(bool); ok {
				fmt.Fprintf(w, "  Healthy:     %v\n", healthy)
			}
			if msg, ok := health["message"].(string); ok && msg != "" {
				fmt.Fprintf(w, "  Health Msg:   %s\n", msg)
			}
		}
	case "session":
		if phase, ok := status["phase"].(string); ok {
			fmt.Fprintf(w, "  Phase:       %s\n", phase)
		}
		if active, ok := status["active"].(bool); ok {
			fmt.Fprintf(w, "  Active:      %v\n", active)
		}
		if expiresAt, ok := status["expiresAt"].(string); ok && expiresAt != "" {
			fmt.Fprintf(w, "  Expires:     %s\n", formatTime(expiresAt))
		}
	case "hive":
		if activeDrones, ok := status["activeDrones"].(float64); ok {
			fmt.Fprintf(w, "  Active Drones: %d\n", int(activeDrones))
		}
		if totalDrones, ok := status["totalDrones"].(float64); ok {
			fmt.Fprintf(w, "  Total Drones:  %d\n", int(totalDrones))
		}
	}
}

// printSpec prints spec information based on resource type.
func printSpec(w *os.File, spec map[string]interface{}, resourceType string) {
	switch resourceType {
	case "agentspec":
		if runtime, ok := spec["runtime"].(map[string]interface{}); ok {
			fmt.Fprintln(w, "  Runtime:")
			if command, ok := runtime["command"].([]interface{}); ok {
				cmdStr := strings.Join(interfaceSliceToStringSlice(command), " ")
				fmt.Fprintf(w, "    Command:    %s\n", cmdStr)
			}
			if workingDir, ok := runtime["workingDir"].(string); ok && workingDir != "" {
				fmt.Fprintf(w, "    WorkingDir: %s\n", workingDir)
			}
			if env, ok := runtime["env"].([]interface{}); ok && len(env) > 0 {
				fmt.Fprintln(w, "    Environment:")
				for _, e := range env {
					if envVar, ok := e.(map[string]interface{}); ok {
						name := getString(envVar, "name")
						value := getString(envVar, "value")
						if value != "" {
							fmt.Fprintf(w, "      %s=%s\n", name, value)
						} else {
							fmt.Fprintf(w, "      %s=<from secret>\n", name)
						}
					}
				}
			}
		}
		if iface, ok := spec["interface"].(map[string]interface{}); ok {
			fmt.Fprintln(w, "  Interface:")
			if typ, ok := iface["type"].(string); ok {
				fmt.Fprintf(w, "    Type:       %s\n", typ)
			}
			if port, ok := iface["port"].(float64); ok {
				fmt.Fprintf(w, "    Port:       %d\n", int(port))
			}
			if healthPath, ok := iface["healthPath"].(string); ok && healthPath != "" {
				fmt.Fprintf(w, "    HealthPath: %s\n", healthPath)
			}
		}
		if resources, ok := spec["resources"].(map[string]interface{}); ok {
			fmt.Fprintln(w, "  Resources:")
			if cpu, ok := resources["cpu"].(float64); ok {
				fmt.Fprintf(w, "    CPU:        %d m\n", int(cpu))
			}
			if memory, ok := resources["memory"].(float64); ok {
				fmt.Fprintf(w, "    Memory:     %d bytes\n", int64(memory))
			}
		}
		if scaling, ok := spec["scaling"].(map[string]interface{}); ok {
			fmt.Fprintln(w, "  Scaling:")
			if minReplicas, ok := scaling["minReplicas"].(float64); ok {
				fmt.Fprintf(w, "    MinReplicas: %d\n", int(minReplicas))
			}
			if maxReplicas, ok := scaling["maxReplicas"].(float64); ok {
				fmt.Fprintf(w, "    MaxReplicas: %d\n", int(maxReplicas))
			}
			if targetLatency, ok := scaling["targetLatencyMs"].(float64); ok {
				fmt.Fprintf(w, "    TargetLatency: %d ms\n", int(targetLatency))
			}
		}
		if guardrails, ok := spec["guardrails"].(map[string]interface{}); ok {
			fmt.Fprintln(w, "  Guardrails:")
			if rateLimit, ok := guardrails["rateLimitPerMinute"].(float64); ok {
				fmt.Fprintf(w, "    RateLimit:  %d/min\n", int(rateLimit))
			}
			if timeout, ok := guardrails["responseTimeoutSeconds"].(float64); ok {
				fmt.Fprintf(w, "    Timeout:    %d s\n", int(timeout))
			}
			if tokenBudget, ok := guardrails["tokenBudgetPerSession"].(float64); ok {
				fmt.Fprintf(w, "    TokenBudget: %d/session\n", int64(tokenBudget))
			}
		}
	case "hive":
		if pattern, ok := spec["pattern"].(string); ok && pattern != "" {
			fmt.Fprintf(w, "  Pattern:     %s\n", pattern)
		}
		if poolMode, ok := spec["poolMode"].(string); ok && poolMode != "" {
			fmt.Fprintf(w, "  PoolMode:    %s\n", poolMode)
		}
		if warmPoolSize, ok := spec["warmPoolSize"].(float64); ok {
			fmt.Fprintf(w, "  WarmPoolSize: %d\n", int(warmPoolSize))
		}
		if stages, ok := spec["stages"].([]interface{}); ok && len(stages) > 0 {
			fmt.Fprintln(w, "  Stages:")
			for i, stage := range stages {
				if s, ok := stage.(map[string]interface{}); ok {
					name := getString(s, "name")
					agentRef := getString(s, "agentRef")
					replicas := getFloat64(s, "replicas")
					fmt.Fprintf(w, "    [%d] %s (Agent: %s, Replicas: %d)\n", i+1, name, agentRef, int(replicas))
				}
			}
		}
		if webhook, ok := spec["webhook"].(map[string]interface{}); ok {
			if url, ok := webhook["url"].(string); ok && url != "" {
				fmt.Fprintf(w, "  Webhook URL: %s\n", url)
			}
		}
	case "cell":
		if quota, ok := spec["resourceQuota"].(map[string]interface{}); ok {
			fmt.Fprintln(w, "  Resource Quota:")
			if maxHives, ok := quota["maxHives"].(float64); ok && maxHives > 0 {
				fmt.Fprintf(w, "    MaxHives:        %d\n", int(maxHives))
			}
			if maxDrones, ok := quota["maxTotalDrones"].(float64); ok && maxDrones > 0 {
				fmt.Fprintf(w, "    MaxTotalDrones:  %d\n", int(maxDrones))
			}
			if maxMemory, ok := quota["maxMemoryGB"].(float64); ok && maxMemory > 0 {
				fmt.Fprintf(w, "    MaxMemoryGB:     %d\n", int(maxMemory))
			}
		}
	case "secret":
		if data, ok := spec["data"].(map[string]interface{}); ok {
			fmt.Fprintln(w, "  Data Keys:")
			for k := range data {
				fmt.Fprintf(w, "    %s\n", k)
			}
		}
	case "drone":
		if specObj, ok := spec["spec"].(map[string]interface{}); ok {
			if agentSpecName, ok := specObj["name"].(string); ok {
				fmt.Fprintf(w, "  AgentSpec:   %s\n", agentSpecName)
			}
		}
	}
}

// Helper functions
func interfaceSliceToStringSlice(slice []interface{}) []string {
	result := make([]string, len(slice))
	for i, v := range slice {
		result[i] = fmt.Sprintf("%v", v)
	}
	return result
}

func getFloat64(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}
