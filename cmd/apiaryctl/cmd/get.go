package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Display one or many resources",
	Long: `Display one or many resources from Apiary.

Resource types:
  cells, cell
  agentspecs, agentspec
  hives, hive
  secrets, secret
  drones, drone
  sessions, session

Examples:
  # List all Hives in a namespace
  apiaryctl get hives -n default

  # Get a specific Hive
  apiaryctl get hive my-hive -n default

  # List all AgentSpecs
  apiaryctl get agentspecs -n default

  # Get resources in YAML format
  apiaryctl get hives -n default -o yaml

  # Get resources with label selector
  apiaryctl get hives -n default -l app=myapp

  # Watch resources for changes
  apiaryctl get hives -n default -w`,
	RunE: runGet,
	ValidArgs: []string{"cells", "cell", "agentspecs", "agentspec", "hives", "hive", "secrets", "secret", "drones", "drone", "sessions", "session"},
}

var (
	getSelector string
	getWatch    bool
)

func init() {
	rootCmd.AddCommand(getCmd)
	getCmd.Flags().StringVarP(&getSelector, "selector", "l", "", "Selector (label query) to filter on")
	getCmd.Flags().BoolVarP(&getWatch, "watch", "w", false, "Watch for changes")
}

func runGet(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("resource type is required")
	}

	client := NewAPIClient()
	namespace := GetNamespace()
	outputFormat := GetOutputFormat()

	// Parse resource type and name
	resourceType := strings.ToLower(args[0])
	var resourceName string
	if len(args) > 1 {
		resourceName = args[1]
	}

	// Normalize resource type (handle singular/plural)
	resourceType = normalizeResourceType(resourceType)

	// Build API path
	var apiPath string
	var err error
	if resourceName != "" {
		// Get single resource
		apiPath, err = getGetPath(resourceType, resourceName, namespace)
		if err != nil {
			return err
		}

		if getWatch {
			return fmt.Errorf("watch mode is not supported for single resources")
		}

		// Get resource
		resp, err := client.Get(apiPath)
		if err != nil {
			return fmt.Errorf("failed to get resource: %w", err)
		}

		var resource interface{}
		if err := client.HandleResponse(resp, &resource); err != nil {
			return err
		}

		// Print resource
		return PrintResource(os.Stdout, resource, outputFormat)
	} else {
		// List resources
		apiPath, err = getListPath(resourceType, namespace)
		if err != nil {
			return err
		}

		if getWatch {
			return watchResources(cmd, client, apiPath, resourceType, outputFormat)
		}

		// List resources
		resp, err := client.Get(apiPath)
		if err != nil {
			return fmt.Errorf("failed to list resources: %w", err)
		}

		var resources []interface{}
		if err := client.HandleResponse(resp, &resources); err != nil {
			return err
		}

		// Apply label selector if provided
		if getSelector != "" {
			resources = filterBySelector(resources, getSelector)
		}

		// Print resources
		if outputFormat == OutputFormatTable {
			return printResourceTable(os.Stdout, resources, resourceType)
		}
		return PrintList(os.Stdout, resources, outputFormat)
	}
}

// normalizeResourceType converts resource type to standard form.
func normalizeResourceType(resourceType string) string {
	switch resourceType {
	case "cell", "cells":
		return "cell"
	case "agentspec", "agentspecs":
		return "agentspec"
	case "hive", "hives":
		return "hive"
	case "secret", "secrets":
		return "secret"
	case "drone", "drones":
		return "drone"
	case "session", "sessions":
		return "session"
	default:
		return resourceType
	}
}

// getGetPath returns the API path for getting a single resource.
func getGetPath(resourceType, name, namespace string) (string, error) {
	switch resourceType {
	case "cell":
		return fmt.Sprintf("/api/v1/cells/%s", name), nil
	case "agentspec":
		return fmt.Sprintf("/api/v1/cells/%s/agentspecs/%s", namespace, name), nil
	case "hive":
		return fmt.Sprintf("/api/v1/cells/%s/hives/%s", namespace, name), nil
	case "secret":
		return fmt.Sprintf("/api/v1/cells/%s/secrets/%s", namespace, name), nil
	case "drone":
		return fmt.Sprintf("/api/v1/cells/%s/drones/%s", namespace, name), nil
	case "session":
		return fmt.Sprintf("/api/v1/cells/%s/sessions/%s", namespace, name), nil
	default:
		return "", fmt.Errorf("unsupported resource type: %s", resourceType)
	}
}

// getListPath returns the API path for listing resources.
func getListPath(resourceType, namespace string) (string, error) {
	switch resourceType {
	case "cell":
		return "/api/v1/cells", nil
	case "agentspec":
		return fmt.Sprintf("/api/v1/cells/%s/agentspecs", namespace), nil
	case "hive":
		return fmt.Sprintf("/api/v1/cells/%s/hives", namespace), nil
	case "secret":
		return fmt.Sprintf("/api/v1/cells/%s/secrets", namespace), nil
	case "drone":
		return fmt.Sprintf("/api/v1/cells/%s/drones", namespace), nil
	case "session":
		return fmt.Sprintf("/api/v1/cells/%s/sessions", namespace), nil
	default:
		return "", fmt.Errorf("unsupported resource type: %s", resourceType)
	}
}

// filterBySelector filters resources by label selector.
func filterBySelector(resources []interface{}, selector string) []interface{} {
	// Parse selector (simple key=value format for now)
	parts := strings.Split(selector, "=")
	if len(parts) != 2 {
		return resources // Invalid selector, return all
	}
	key := parts[0]
	value := parts[1]

	var filtered []interface{}
	for _, resource := range resources {
		if matchesSelector(resource, key, value) {
			filtered = append(filtered, resource)
		}
	}
	return filtered
}

// matchesSelector checks if a resource matches a label selector.
func matchesSelector(resource interface{}, key, value string) bool {
	// Convert to JSON to extract labels
	data, err := json.Marshal(resource)
	if err != nil {
		return false
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return false
	}
	metadata, ok := m["metadata"].(map[string]interface{})
	if !ok {
		return false
	}
	labels, ok := metadata["labels"].(map[string]interface{})
	if !ok {
		return false
	}
	labelValue, ok := labels[key].(string)
	return ok && labelValue == value
}

// printResourceTable prints resources in table format.
func printResourceTable(w io.Writer, resources []interface{}, resourceType string) error {
	if len(resources) == 0 {
		fmt.Fprintln(w, "No resources found.")
		return nil
	}

	tw := NewTabWriter(w)
	defer tw.Flush()

	// Print header based on resource type
	switch resourceType {
	case "cell":
		fmt.Fprintln(tw, "NAME\tCREATED")
		for _, r := range resources {
			name, created := extractCellFields(r)
			fmt.Fprintf(tw, "%s\t%s\n", name, created)
		}
	case "agentspec", "hive", "secret":
		fmt.Fprintln(tw, "NAME\tNAMESPACE\tCREATED")
		for _, r := range resources {
			name, ns, created := extractNamespacedFields(r)
			fmt.Fprintf(tw, "%s\t%s\t%s\n", name, ns, created)
		}
	case "drone", "session":
		fmt.Fprintln(tw, "NAME\tNAMESPACE\tSTATUS\tCREATED")
		for _, r := range resources {
			name, ns, status, created := extractStatusFields(r)
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", name, ns, status, created)
		}
	default:
		// Fallback to JSON
		return PrintList(w, resources, OutputFormatJSON)
	}

	return nil
}

// extractCellFields extracts name and created time from a Cell resource.
func extractCellFields(resource interface{}) (string, string) {
	data, _ := json.Marshal(resource)
	var m map[string]interface{}
	json.Unmarshal(data, &m)
	metadata := m["metadata"].(map[string]interface{})
	name := getString(metadata, "name")
	created := getString(metadata, "createdAt")
	return name, formatTime(created)
}

// extractNamespacedFields extracts name, namespace, and created time.
func extractNamespacedFields(resource interface{}) (string, string, string) {
	data, _ := json.Marshal(resource)
	var m map[string]interface{}
	json.Unmarshal(data, &m)
	metadata := m["metadata"].(map[string]interface{})
	name := getString(metadata, "name")
	ns := getString(metadata, "namespace")
	created := getString(metadata, "createdAt")
	return name, ns, formatTime(created)
}

// extractStatusFields extracts name, namespace, status, and created time.
func extractStatusFields(resource interface{}) (string, string, string, string) {
	data, _ := json.Marshal(resource)
	var m map[string]interface{}
	json.Unmarshal(data, &m)
	metadata := m["metadata"].(map[string]interface{})
	name := getString(metadata, "name")
	ns := getString(metadata, "namespace")
	created := getString(metadata, "createdAt")
	
	status := "Unknown"
	if s, ok := m["status"].(map[string]interface{}); ok {
		if phase, ok := s["phase"].(string); ok {
			status = phase
		}
	}
	
	return name, ns, status, formatTime(created)
}

// getString safely gets a string value from a map.
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// formatTime formats a timestamp string for display.
func formatTime(timeStr string) string {
	if timeStr == "" {
		return "<unknown>"
	}
	// For now, just return the time as-is or a shortened version
	// In a full implementation, we'd parse and format it nicely
	if len(timeStr) > 19 {
		return timeStr[:19] // Return date and time part
	}
	return timeStr
}

// watchResources watches resources for changes (simplified implementation).
func watchResources(cmd *cobra.Command, client *APIClient, apiPath string, resourceType string, outputFormat OutputFormat) error {
	// TODO: Implement proper watch mode using Server-Sent Events or polling
	// For now, just do a single poll
	cmd.Println("Watch mode - polling for changes (full watch implementation pending)")
	
	resp, err := client.Get(apiPath)
	if err != nil {
		return fmt.Errorf("failed to list resources: %w", err)
	}

	var resources []interface{}
	if err := client.HandleResponse(resp, &resources); err != nil {
		return err
	}

	if outputFormat == OutputFormatTable {
		return printResourceTable(os.Stdout, resources, resourceType)
	}
	return PrintList(os.Stdout, resources, outputFormat)
}
