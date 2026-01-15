package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// OutputFormat represents the output format type.
type OutputFormat string

const (
	OutputFormatYAML OutputFormat = "yaml"
	OutputFormatJSON OutputFormat = "json"
	OutputFormatWide OutputFormat = "wide"
	OutputFormatName OutputFormat = "name"
	OutputFormatTable OutputFormat = "table" // Default
)

// GetOutputFormat returns the output format from viper config or flag.
func GetOutputFormat() OutputFormat {
	format := strings.ToLower(viper.GetString("output"))
	switch format {
	case "yaml", "y":
		return OutputFormatYAML
	case "json", "j":
		return OutputFormatJSON
	case "wide", "w":
		return OutputFormatWide
	case "name", "n":
		return OutputFormatName
	case "table", "t", "":
		return OutputFormatTable
	default:
		return OutputFormatTable
	}
}

// PrintResource prints a resource in the specified format.
func PrintResource(w io.Writer, resource interface{}, format OutputFormat) error {
	switch format {
	case OutputFormatYAML:
		return printYAML(w, resource)
	case OutputFormatJSON:
		return printJSON(w, resource)
	case OutputFormatName:
		return printName(w, resource)
	case OutputFormatTable, OutputFormatWide:
		// Table format is resource-specific, handled by individual commands
		return printJSON(w, resource) // Fallback to JSON
	default:
		return printJSON(w, resource)
	}
}

// PrintList prints a list of resources in the specified format.
func PrintList(w io.Writer, resources interface{}, format OutputFormat) error {
	switch format {
	case OutputFormatYAML:
		return printYAML(w, resources)
	case OutputFormatJSON:
		return printJSON(w, resources)
	case OutputFormatName:
		return printNameList(w, resources)
	case OutputFormatTable, OutputFormatWide:
		// Table format is resource-specific, handled by individual commands
		return printJSON(w, resources) // Fallback to JSON
	default:
		return printJSON(w, resources)
	}
}

func printYAML(w io.Writer, v interface{}) error {
	encoder := yaml.NewEncoder(w)
	encoder.SetIndent(2)
	defer encoder.Close()
	return encoder.Encode(v)
}

func printJSON(w io.Writer, v interface{}) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

func printName(w io.Writer, v interface{}) error {
	// Extract name from resource (assumes it has a GetName() method or similar)
	// This is a simplified version - individual commands should implement proper name extraction
	if str, ok := v.(fmt.Stringer); ok {
		fmt.Fprintln(w, str.String())
		return nil
	}
	// Fallback: try to extract name from JSON
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	if metadata, ok := m["metadata"].(map[string]interface{}); ok {
		if name, ok := metadata["name"].(string); ok {
			fmt.Fprintln(w, name)
			return nil
		}
	}
	return fmt.Errorf("unable to extract name from resource")
}

func printNameList(w io.Writer, v interface{}) error {
	// Extract names from list of resources
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var list []interface{}
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	for _, item := range list {
		if m, ok := item.(map[string]interface{}); ok {
			if metadata, ok := m["metadata"].(map[string]interface{}); ok {
				if name, ok := metadata["name"].(string); ok {
					fmt.Fprintln(w, name)
				}
			}
		}
	}
	return nil
}

// NewTabWriter creates a new tabwriter for table output.
func NewTabWriter(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
}

// PrintError prints an error message to stderr.
func PrintError(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
}

// PrintSuccess prints a success message.
func PrintSuccess(msg string) {
	fmt.Fprintln(os.Stdout, msg)
}
