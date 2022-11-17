package cmd

import (
	"os"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/shipatlas/ecs-toolkit/pkg"
	"github.com/shipatlas/ecs-toolkit/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	log "github.com/sirupsen/logrus"
)

type rootOptions struct {
	configFile string
	logLevel   string
}

var toolConfig = pkg.Config{}

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
		log.Error(err)
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
	log.Infof("using config file: %s", rootCmdOptions.configFile)

	// If a config file is found, read it in.
	log.Infof("reading %s config file", viper.ConfigFileUsed())
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("unable to read %s config file: %v", viper.ConfigFileUsed(), err)
	}

	log.Info("parsing config file")
	if err := viper.Unmarshal(&toolConfig); err != nil {
		log.Fatalf("unable to parse config file: %v", err)
	}

	log.Info("validating config file")
	validate := validator.New()
	err := validate.Struct(&toolConfig)
	if err != nil {
		if _, ok := err.(*validator.InvalidValidationError); ok {
			log.Error(strings.ToLower(err.Error()))
		}

		for _, err := range err.(validator.ValidationErrors) {
			log.Error(strings.ToLower(err.Error()))
		}

		log.Fatal("unable to validate config file")
	}
}

func initLogging() {
	utils.SetLogLevel(rootCmdOptions.logLevel)
}
