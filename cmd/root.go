package cmd

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/shipatlas/ecs-toolkit/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type rootOptions struct {
	configFile string
	logLevel   string
}

var (
	rootCmdLong = utils.LongDesc(`
		Tool to make it easier to work with AWS ECS.`)

	rootCmdExamples = utils.Examples(`
		# Set the configuration file to use
		ecs-toolkit --config=/some/other/path/.ecs-toolkit.yml
		
		# Set the logging level i.e. in order: trace, debug, info, warn, error, fatal, panic
		ecs-toolkit --log-level=debug`)

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
		log.Error().Err(err).Msg("")
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initLogging, initConfig)

	// Persistent flags, which, will be global for the application.
	rootCmd.PersistentFlags().StringVarP(&rootCmdOptions.configFile, "config", "c", ".ecs-toolkit.yml", "path to configuration file")
	rootCmd.PersistentFlags().StringVarP(&rootCmdOptions.logLevel, "log-level", "l", "warn", "logging level i.e. "+strings.Join(utils.LogLevels, "|"))
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
	log.Info().Msgf("using config file: %s", rootCmdOptions.configFile)

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		log.Info().Msgf("loaded %s config file", viper.ConfigFileUsed())
	} else {
		log.Fatal().Err(err).Msgf("unable to load %s config file", viper.ConfigFileUsed())
	}
}

func initLogging() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	utils.SetLogLevel(rootCmdOptions.logLevel)
}
