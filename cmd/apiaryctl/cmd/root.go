package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile     string
	apiServer   string
	namespace   string
	output      string
	userID      string
	userName    string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "apiaryctl",
	Short: "Apiary command-line tool for managing AI agent orchestration",
	Long: `apiaryctl is a command-line tool for managing Apiary resources.

Apiary is a general-purpose orchestration platform for AI agents, designed with
the same philosophy that Kubernetes brought to container orchestration.

Examples:
  # List all Hives in a namespace
  apiaryctl get hives -n default

  # Apply a manifest
  apiaryctl apply -f manifest.yaml

  # Describe a resource
  apiaryctl describe hive my-hive -n default

  # View logs from a Drone
  apiaryctl logs drone/my-drone -n default`,
	Version: "0.1.0",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.apiaryctl.yaml)")
	rootCmd.PersistentFlags().StringVarP(&apiServer, "server", "s", "http://localhost:8080", "API server address")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "default", "namespace (Cell) to use")
	rootCmd.PersistentFlags().StringVarP(&output, "output", "o", "", "Output format (yaml, json, wide, name)")
	rootCmd.PersistentFlags().StringVar(&userID, "user-id", "", "User ID for authentication (or set APIARY_USER_ID env var)")
	rootCmd.PersistentFlags().StringVar(&userName, "user-name", "", "User name for authentication (or set APIARY_USER_NAME env var)")

	// Bind flags to viper
	viper.BindPFlag("server", rootCmd.PersistentFlags().Lookup("server"))
	viper.BindPFlag("namespace", rootCmd.PersistentFlags().Lookup("namespace"))
	viper.BindPFlag("output", rootCmd.PersistentFlags().Lookup("output"))
	viper.BindPFlag("user-id", rootCmd.PersistentFlags().Lookup("user-id"))
	viper.BindPFlag("user-name", rootCmd.PersistentFlags().Lookup("user-name"))

	// Environment variables
	viper.SetEnvPrefix("APIARY")
	viper.AutomaticEnv()
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".apiaryctl" (without extension).
		viper.AddConfigPath(home)
		viper.AddConfigPath(".")
		viper.SetConfigType("yaml")
		viper.SetConfigName(".apiaryctl")
	}

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}
