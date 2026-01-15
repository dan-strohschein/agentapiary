package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Print logs from a Drone",
	Long: `Print logs from a Drone's Keeper and agent process.

The logs command retrieves logs from a Drone. Logs include both Keeper
sidecar logs and agent process output.

Examples:
  # View logs from a Drone
  apiaryctl logs drone/my-drone -n default

  # Follow logs (stream)
  apiaryctl logs drone/my-drone -n default -f

  # Show last 100 lines
  apiaryctl logs drone/my-drone -n default --tail=100

  # Show logs since 1 hour ago
  apiaryctl logs drone/my-drone -n default --since=1h`,
	RunE: runLogs,
}

var (
	logsFollow bool
	logsTail   int
	logsSince  string
)

func init() {
	rootCmd.AddCommand(logsCmd)
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
	logsCmd.Flags().IntVar(&logsTail, "tail", -1, "Number of lines to show from the end of logs")
	logsCmd.Flags().StringVar(&logsSince, "since", "", "Show logs since timestamp (e.g. 1h, 30m, 2021-01-01T00:00:00Z)")
}

func runLogs(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("drone name is required (format: drone/<name>)")
	}

	// Parse drone name (format: drone/<name> or just <name>)
	droneName := args[0]
	if strings.HasPrefix(droneName, "drone/") {
		droneName = strings.TrimPrefix(droneName, "drone/")
	}

	client := NewAPIClient()
	namespace := GetNamespace()

	// Build logs API path
	logsPath := fmt.Sprintf("/api/v1/cells/%s/drones/%s/logs", namespace, droneName)
	params := []string{}
	if logsTail > 0 {
		params = append(params, fmt.Sprintf("tail=%d", logsTail))
	}
	if logsSince != "" {
		params = append(params, fmt.Sprintf("since=%s", logsSince))
	}
	if logsFollow {
		params = append(params, "follow=true")
	}
	if len(params) > 0 {
		logsPath += "?" + strings.Join(params, "&")
	}

	// Fetch logs from API
	if logsFollow {
		return streamLogs(cmd, logsPath, client)
	}

	return fetchLogs(cmd, logsPath, client)
}

// fetchLogs fetches logs once and prints them.
func fetchLogs(cmd *cobra.Command, path string, client *APIClient) error {
	resp, err := client.Get(path)
	if err != nil {
		return fmt.Errorf("failed to fetch logs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to fetch logs (status %d): %s", resp.StatusCode, string(body))
	}

	// Read and print logs
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		cmd.Println(scanner.Text())
	}

	return scanner.Err()
}

// streamLogs streams logs continuously.
func streamLogs(cmd *cobra.Command, path string, client *APIClient) error {
	// For streaming, we'd typically use Server-Sent Events or similar
	// For MVP, implement polling-based streaming
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lastLineCount := 0
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			resp, err := client.Get(path)
			if err != nil {
				// Continue on error (Keeper might be restarting)
				continue
			}

			if resp.StatusCode == http.StatusOK {
				scanner := bufio.NewScanner(resp.Body)
				lineCount := 0
				for scanner.Scan() {
					lineCount++
					// Only print new lines
					if lineCount > lastLineCount {
						cmd.Println(scanner.Text())
					}
				}
				lastLineCount = lineCount
			}
			resp.Body.Close()
		}
	}
}
