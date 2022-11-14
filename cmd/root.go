package cmd

import (
	"fmt"
	"os"

	"github.com/shipatlas/ecs-toolkit/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type rootOptions struct {
	configFile string
}

var (
	rootCmdLong = utils.LongDesc(`
		Tool to make it easier to work with AWS ECS.`)

	rootCmdExamples = utils.Examples(`
		# Set the configuration file to use
		ecs-toolkit --config=/some/other/path/.ecs-toolkit.yml`)

	rootCmdOptions = &rootOptions{}
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:           "ecs-toolkit",
	Short:         "Tool to make it easier to work with AWS ECS.",
	Long:          rootCmdLong,
	Example:       rootCmdExamples,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute adds all child commands to the root command and sets flags
// appropriately. This is called by main.main(). It only needs to happen once to
// the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Persistent flags, which, will be global for the application.
	rootCmd.PersistentFlags().StringVarP(&rootCmdOptions.configFile, "config", "c", ".ecs-toolkit.yml", "path to configuration file")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if rootCmdOptions.configFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(rootCmdOptions.configFile)
	} else {
		// Search config in current directory with name ".ecs-toolkit" (without
		// extension).
		viper.AddConfigPath(".")
		viper.SetConfigType("yml")
		viper.SetConfigName(".ecs-toolkit")
	}

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}
