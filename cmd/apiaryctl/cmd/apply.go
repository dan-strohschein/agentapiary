package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentapiary/apiary/pkg/manifest"
	"github.com/spf13/cobra"
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply a manifest to create or update resources",
	Long: `Apply a manifest file to create or update resources in Apiary.

The apply command creates resources if they don't exist, or updates them if they do.
This is an idempotent operation.

Examples:
  # Apply a single manifest file
  apiaryctl apply -f manifest.yaml

  # Apply multiple manifest files
  apiaryctl apply -f manifest1.yaml -f manifest2.yaml

  # Apply with dry-run to validate without creating
  apiaryctl apply -f manifest.yaml --dry-run`,
	RunE: runApply,
}

var (
	applyFiles   []string
	applyDryRun  bool
	applyRecurse bool
)

func init() {
	rootCmd.AddCommand(applyCmd)
	applyCmd.Flags().StringArrayVarP(&applyFiles, "filename", "f", []string{}, "Manifest file(s) to apply")
	applyCmd.Flags().BoolVar(&applyDryRun, "dry-run", false, "Validate manifest without applying")
	applyCmd.Flags().BoolVarP(&applyRecurse, "recursive", "R", false, "Process the directory used in -f, --filename recursively")
	applyCmd.MarkFlagRequired("filename")
}

func runApply(cmd *cobra.Command, args []string) error {
	client := NewAPIClient()
	namespace := GetNamespace()

	// Collect all files to process
	var filesToProcess []string
	for _, file := range applyFiles {
		info, err := os.Stat(file)
		if err != nil {
			return fmt.Errorf("failed to stat file %s: %w", file, err)
		}

		if info.IsDir() {
			if !applyRecurse {
				return fmt.Errorf("%s is a directory (use -R to process recursively)", file)
			}
			// Walk directory
			err := filepath.Walk(file, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !info.IsDir() && (strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".json")) {
					filesToProcess = append(filesToProcess, path)
				}
				return nil
			})
			if err != nil {
				return fmt.Errorf("failed to walk directory %s: %w", file, err)
			}
		} else {
			filesToProcess = append(filesToProcess, file)
		}
	}

	if len(filesToProcess) == 0 {
		return fmt.Errorf("no manifest files found")
	}

	// Process each file
	var appliedCount, updatedCount, errorCount int
	for _, file := range filesToProcess {
		cmd.Printf("Processing file: %s\n", file)

		// Parse and validate manifest
		m, validationErrors, err := manifest.ParseAndValidateFile(file)
		if err != nil {
			PrintError(fmt.Errorf("failed to parse %s: %w", file, err))
			errorCount++
			continue
		}

		if len(validationErrors) > 0 {
			PrintError(fmt.Errorf("validation errors in %s:", file))
			for _, ve := range validationErrors {
				cmd.Printf("  - %s\n", ve.Error())
			}
			errorCount++
			continue
		}

		// Use namespace from manifest or flag
		manifestNamespace := m.Metadata.Namespace
		if manifestNamespace == "" {
			manifestNamespace = namespace
		}

		if applyDryRun {
			cmd.Printf("  ✓ %s/%s validated (dry-run)\n", m.Kind, m.Metadata.Name)
			continue
		}

		// Determine API path based on resource kind
		apiPath, err := getAPIPath(m.Kind, m.Metadata.Name, manifestNamespace)
		if err != nil {
			PrintError(fmt.Errorf("failed to determine API path for %s: %w", file, err))
			errorCount++
			continue
		}

		// Check if resource exists
		resp, err := client.Get(apiPath)
		resourceExists := false
		if err == nil && resp != nil {
			// Check if response is successful (200-299)
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				resourceExists = true
			}
			resp.Body.Close()
		} else if err != nil {
			// If error is not 404, it's a real error
			if resp != nil && resp.StatusCode == 404 {
				// 404 means resource doesn't exist, which is fine
				resp.Body.Close()
			} else {
				PrintError(fmt.Errorf("failed to check if %s/%s exists: %w", m.Kind, m.Metadata.Name, err))
				errorCount++
				continue
			}
		}

		if resourceExists {
			// Resource exists, update it
			updateResp, err := client.Put(apiPath, m.Resource)
			if err != nil {
				PrintError(fmt.Errorf("failed to update %s/%s: %w", m.Kind, m.Metadata.Name, err))
				errorCount++
				continue
			}
			if err := client.HandleResponse(updateResp, nil); err != nil {
				PrintError(fmt.Errorf("failed to update %s/%s: %w", m.Kind, m.Metadata.Name, err))
				errorCount++
				continue
			}
			cmd.Printf("  ✓ %s/%s updated\n", m.Kind, m.Metadata.Name)
			updatedCount++
		} else {
			// Resource doesn't exist, create it
			createPath, err := getCreatePath(m.Kind, manifestNamespace)
			if err != nil {
				PrintError(fmt.Errorf("failed to determine create path for %s: %w", file, err))
				errorCount++
				continue
			}
			createResp, err := client.Post(createPath, m.Resource)
			if err != nil {
				PrintError(fmt.Errorf("failed to create %s/%s: %w", m.Kind, m.Metadata.Name, err))
				errorCount++
				continue
			}
			if err := client.HandleResponse(createResp, nil); err != nil {
				PrintError(fmt.Errorf("failed to create %s/%s: %w", m.Kind, m.Metadata.Name, err))
				errorCount++
				continue
			}
			cmd.Printf("  ✓ %s/%s created\n", m.Kind, m.Metadata.Name)
			appliedCount++
		}
	}

	// Summary
	if applyDryRun {
		cmd.Printf("\n✓ All manifests validated (dry-run)\n")
	} else {
		cmd.Printf("\nSummary: %d created, %d updated", appliedCount, updatedCount)
		if errorCount > 0 {
			cmd.Printf(", %d errors", errorCount)
		}
		cmd.Println()
	}

	if errorCount > 0 {
		return fmt.Errorf("encountered %d errors", errorCount)
	}

	return nil
}

// getAPIPath returns the API path for getting/updating a resource.
func getAPIPath(kind, name, namespace string) (string, error) {
	kindLower := strings.ToLower(kind)
	
	// Handle non-namespaced resources
	if kind == "Cell" {
		return fmt.Sprintf("/api/v1/cells/%s", name), nil
	}

	// Handle namespaced resources
	switch kindLower {
	case "agentspec":
		return fmt.Sprintf("/api/v1/cells/%s/agentspecs/%s", namespace, name), nil
	case "hive":
		return fmt.Sprintf("/api/v1/cells/%s/hives/%s", namespace, name), nil
	case "secret":
		return fmt.Sprintf("/api/v1/cells/%s/secrets/%s", namespace, name), nil
	default:
		return "", fmt.Errorf("unsupported resource kind: %s", kind)
	}
}

// getCreatePath returns the API path for creating a resource.
func getCreatePath(kind, namespace string) (string, error) {
	kindLower := strings.ToLower(kind)
	
	// Handle non-namespaced resources
	if kind == "Cell" {
		return "/api/v1/cells", nil
	}

	// Handle namespaced resources
	switch kindLower {
	case "agentspec":
		return fmt.Sprintf("/api/v1/cells/%s/agentspecs", namespace), nil
	case "hive":
		return fmt.Sprintf("/api/v1/cells/%s/hives", namespace), nil
	case "secret":
		return fmt.Sprintf("/api/v1/cells/%s/secrets", namespace), nil
	default:
		return "", fmt.Errorf("unsupported resource kind: %s", kind)
	}
}
