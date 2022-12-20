/*
Copyright 2022 King'ori Maina

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
	rootCmd.PersistentFlags().StringVarP(&rootCmdOptions.logLevel, "log-level", "l", "info", "logging level i.e. "+strings.Join(utils.LogLevels, "|"))
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

	log.Debugf("parsing %s config file", viper.ConfigFileUsed())
	if err := viper.Unmarshal(&toolConfig); err != nil {
		log.Fatalf("unable to parse %s config file: %v", viper.ConfigFileUsed(), err)
	}

	log.Debugf("validating %s config file", viper.ConfigFileUsed())
	validate := validator.New()
	err := validate.Struct(&toolConfig)
	if err != nil {
		if _, ok := err.(*validator.InvalidValidationError); ok {
			log.Error(strings.ToLower(err.Error()))
		}

		for _, err := range err.(validator.ValidationErrors) {
			log.Error(strings.ToLower(err.Error()))
		}

		log.Fatalf("unable to validate %s config file", viper.ConfigFileUsed())
	}
}

func initLogging() {
	utils.SetLogLevel(rootCmdOptions.logLevel)
}
