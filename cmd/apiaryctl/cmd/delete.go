package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/agentapiary/apiary/pkg/manifest"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete resources",
	Long: `Delete resources from Apiary.

Examples:
  # Delete a Hive
  apiaryctl delete hive my-hive -n default

  # Delete an AgentSpec
  apiaryctl delete agentspec my-agent -n default

  # Delete a Drone
  apiaryctl delete drone my-drone -n default

  # Delete with force (skip confirmation)
  apiaryctl delete hive my-hive -n default --force

  # Delete resources from a manifest file
  apiaryctl delete -f manifest.yaml`,
	RunE: runDelete,
}

var (
	deleteForce bool
	deleteFile  string
)

func init() {
	rootCmd.AddCommand(deleteCmd)
	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "Skip confirmation prompt")
	deleteCmd.Flags().StringVarP(&deleteFile, "filename", "f", "", "Delete resources from manifest file")
}

func runDelete(cmd *cobra.Command, args []string) error {
	client := NewAPIClient()
	namespace := GetNamespace()

	var resourcesToDelete []resourceToDelete

	// If filename is provided, parse manifest and delete those resources
	if deleteFile != "" {
		// Parse manifest file
		m, validationErrors, err := manifest.ParseAndValidateFile(deleteFile)
		if err != nil {
			return fmt.Errorf("failed to parse manifest file: %w", err)
		}

		if len(validationErrors) > 0 {
			cmd.Printf("Warning: Validation errors in manifest:\n")
			for _, ve := range validationErrors {
				cmd.Printf("  - %s\n", ve.Error())
			}
		}

		manifestNamespace := m.Metadata.Namespace
		if manifestNamespace == "" {
			manifestNamespace = namespace
		}

		resourcesToDelete = append(resourcesToDelete, resourceToDelete{
			kind:      m.Kind,
			name:      m.Metadata.Name,
			namespace: manifestNamespace,
		})
	} else {
		// Delete from command line arguments
		if len(args) < 2 {
			return fmt.Errorf("resource type and name are required (or use -f to delete from manifest)")
		}

		resourceType := strings.ToLower(args[0])
		resourceName := args[1]

		// Normalize resource type
		resourceType = normalizeResourceType(resourceType)

		resourcesToDelete = append(resourcesToDelete, resourceToDelete{
			kind:      resourceType,
			name:      resourceName,
			namespace: namespace,
		})
	}

	// Check for cascade deletion (dependent resources)
	for _, res := range resourcesToDelete {
		if err := checkCascadeDeletion(cmd, client, res); err != nil {
			return err
		}
	}

	// Confirm deletion (unless --force)
	if !deleteForce {
		cmd.Printf("You are about to delete the following resources:\n")
		for _, res := range resourcesToDelete {
			cmd.Printf("  %s/%s", res.kind, res.name)
			if res.namespace != "" {
				cmd.Printf(" in namespace %s", res.namespace)
			}
			cmd.Println()
		}
		cmd.Print("Are you sure you want to continue? (yes/no): ")

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read confirmation: %w", err)
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "yes" && response != "y" {
			cmd.Println("Deletion cancelled.")
			return nil
		}
	}

	// Delete resources
	var deletedCount, errorCount int
	for _, res := range resourcesToDelete {
		apiPath, err := getDeletePath(res.kind, res.name, res.namespace)
		if err != nil {
			PrintError(fmt.Errorf("failed to determine delete path for %s/%s: %w", res.kind, res.name, err))
			errorCount++
			continue
		}

		resp, err := client.Delete(apiPath)
		if err != nil {
			PrintError(fmt.Errorf("failed to delete %s/%s: %w", res.kind, res.name, err))
			errorCount++
			continue
		}

		if err := client.HandleResponse(resp, nil); err != nil {
			PrintError(fmt.Errorf("failed to delete %s/%s: %w", res.kind, res.name, err))
			errorCount++
			continue
		}

		cmd.Printf("✓ %s/%s deleted\n", res.kind, res.name)
		deletedCount++
	}

	// Summary
	cmd.Printf("\nDeleted %d resource(s)", deletedCount)
	if errorCount > 0 {
		cmd.Printf(", %d error(s)", errorCount)
	}
	cmd.Println()

	if errorCount > 0 {
		return fmt.Errorf("encountered %d errors", errorCount)
	}

	return nil
}

type resourceToDelete struct {
	kind      string
	name      string
	namespace string
}

// getDeletePath returns the API path for deleting a resource.
func getDeletePath(resourceType, name, namespace string) (string, error) {
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

// checkCascadeDeletion checks for dependent resources that will be affected.
func checkCascadeDeletion(cmd *cobra.Command, client *APIClient, res resourceToDelete) error {
	// If deleting an AgentSpec, check for Drones
	if res.kind == "agentspec" {
		// List Drones with label selector for this AgentSpec
		dronesPath := fmt.Sprintf("/api/v1/cells/%s/drones", res.namespace)
		resp, err := client.Get(dronesPath)
		if err == nil && resp != nil {
			if resp.StatusCode == 200 {
				var drones []interface{}
				if err := client.HandleResponse(resp, &drones); err == nil {
					// Filter drones for this AgentSpec
					var matchingDrones []string
					for _, drone := range drones {
						if d, ok := drone.(map[string]interface{}); ok {
							if metadata, ok := d["metadata"].(map[string]interface{}); ok {
								if labels, ok := metadata["labels"].(map[string]interface{}); ok {
									if agentspec, ok := labels["agentspec"].(string); ok && agentspec == res.name {
										if name, ok := metadata["name"].(string); ok {
											matchingDrones = append(matchingDrones, name)
										}
									}
								}
							}
						}
					}
					if len(matchingDrones) > 0 {
						cmd.Printf("Warning: Deleting %s/%s will affect %d Drone(s):\n", res.kind, res.name, len(matchingDrones))
						for _, droneName := range matchingDrones {
							cmd.Printf("  - %s\n", droneName)
						}
						cmd.Println("These Drones will be stopped and removed by the Queen's reconciliation loop.")
					}
				}
			} else {
				resp.Body.Close()
			}
		}
	}

	// If deleting a Hive, check for Sessions
	if res.kind == "hive" {
		sessionsPath := fmt.Sprintf("/api/v1/cells/%s/sessions", res.namespace)
		resp, err := client.Get(sessionsPath)
		if err == nil && resp != nil {
			if resp.StatusCode == 200 {
				var sessions []interface{}
				if err := client.HandleResponse(resp, &sessions); err == nil {
					// Filter sessions for this Hive (check annotations)
					var matchingSessions []string
					for _, session := range sessions {
						if s, ok := session.(map[string]interface{}); ok {
							if metadata, ok := s["metadata"].(map[string]interface{}); ok {
								if annotations, ok := metadata["annotations"].(map[string]interface{}); ok {
									if hiveName, ok := annotations["hive"].(string); ok && hiveName == res.name {
										if name, ok := metadata["name"].(string); ok {
											matchingSessions = append(matchingSessions, name)
										}
									}
								}
							}
						}
					}
					if len(matchingSessions) > 0 {
						cmd.Printf("Warning: Deleting %s/%s will affect %d Session(s):\n", res.kind, res.name, len(matchingSessions))
						for _, sessionName := range matchingSessions {
							cmd.Printf("  - %s\n", sessionName)
						}
					}
				}
			} else {
				resp.Body.Close()
			}
		}
	}

	return nil
}
