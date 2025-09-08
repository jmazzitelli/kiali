package cli

import (
	"os"
	"path/filepath"

	"github.com/kiali/kiali-integration-framework/pkg/types"
	"github.com/kiali/kiali-integration-framework/pkg/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	// Version of the kiali-integration-framework
	Version = "0.1.0"
)

// NewRootCommand creates and returns the root command
func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kiali-integration-framework",
		Short: "Kiali Integration Test Framework",
		Long: `A robust, modular Go-based framework for Kiali integration testing.

This framework replaces the fragile bash-based integration test system with a
maintainable, configurable, and extensible solution.`,
		Version: Version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Initialize configuration
			return initConfig()
		},
	}

	// Add global flags
	rootCmd.PersistentFlags().String("config", "", "config file (default is $HOME/.kiali-integration-framework.yaml)")
	rootCmd.PersistentFlags().String("log-level", "info", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().Bool("verbose", false, "verbose output")

	// Bind flags to viper
	viper.BindPFlag("config", rootCmd.PersistentFlags().Lookup("config"))
	viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level"))
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))

	// Add subcommands
	rootCmd.AddCommand(newInitCommand())
	rootCmd.AddCommand(newUpCommand())
	rootCmd.AddCommand(newRunCommand())
	rootCmd.AddCommand(newDownCommand())
	rootCmd.AddCommand(newStatusCommand())
	rootCmd.AddCommand(newTopologyCommand())
	rootCmd.AddCommand(newClusterCommand())
	rootCmd.AddCommand(newServiceDiscoveryCommand())

	return rootCmd
}

// initConfig initializes the configuration and logging
func initConfig() error {
	logger := utils.GetGlobalLogger()

	// Initialize logging first
	logLevel := utils.LogLevel(viper.GetString("log-level"))
	verbose := viper.GetBool("verbose")

	if err := utils.InitGlobalLogger(logLevel, verbose, ""); err != nil {
		return utils.WrapError(err, utils.ErrCodeInternalError, "failed to initialize logger")
	}

	logger = utils.GetGlobalLogger()
	logger.Infof("Initializing Kiali Integration Framework v%s", Version)

	configFile := viper.GetString("config")

	if configFile != "" {
		// Use config file from the flag
		viper.SetConfigFile(configFile)
		logger.Infof("Using config file from flag: %s", configFile)
	} else {
		// Find home directory
		home, err := os.UserHomeDir()
		if err != nil {
			return utils.WrapError(err, utils.ErrCodeInternalError, "failed to get home directory")
		}

		// Search config in home directory with name ".kiali-integration-framework" (without extension)
		viper.AddConfigPath(home)
		viper.SetConfigName(".kiali-integration-framework")
		logger.Debugf("Searching for config in: %s", home)
	}

	// Read in environment variables that match
	viper.SetEnvPrefix("KIALI_INT")
	viper.AutomaticEnv()

	// If a config file is found, read it in
	if err := viper.ReadInConfig(); err == nil {
		logger.Infof("Using config file: %s", viper.ConfigFileUsed())
	} else {
		if configFile != "" {
			// Config file was specified but couldn't be read
			return utils.WrapError(err, utils.ErrCodeConfigNotFound, "specified config file could not be read")
		}
		// Config file not found, but that's OK for now
		logger.Debug("No config file found, using defaults")
	}

	// Set default values
	setConfigDefaults()

	logger.Info("Configuration initialized successfully")
	return nil
}

// setConfigDefaults sets default configuration values
func setConfigDefaults() {
	viper.SetDefault("global.timeout", "300s")
	viper.SetDefault("global.workingDir", ".")
	viper.SetDefault("global.tempDir", filepath.Join(os.TempDir(), "kiali-integration-framework"))
	viper.SetDefault("cluster.provider", string(types.ClusterProviderKind))
	viper.SetDefault("cluster.version", "1.27.0")
}
