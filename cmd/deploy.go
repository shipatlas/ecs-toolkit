package cmd

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/smithy-go/logging"
	"github.com/shipatlas/ecs-toolkit/utils"
	"github.com/spf13/cobra"

	log "github.com/sirupsen/logrus"
)

type deployOptions struct {
	imageTag string
}

var (
	deployCmdLong = utils.LongDesc(`
		Deploy an application to AWS ECS`)

	deployCmdExamples = utils.Examples(`
		# Deploy new revision of an application
		ecs-toolkit deploy --image-tag=5a853f72`)

	deployCmdOptions = &deployOptions{}
)

// deployCmd represents the deploy command
var deployCmd = &cobra.Command{
	Use:     "deploy",
	Short:   "Deploy an application to AWS ECS.",
	Long:    deployCmdLong,
	Example: deployCmdExamples,
	Run: func(cmd *cobra.Command, args []string) {
		deployCmdOptions.validate()
		deployCmdOptions.run()
	},
}

func init() {
	rootCmd.AddCommand(deployCmd)

	// Local flags, which, will be global for the application.
	deployCmd.Flags().StringVarP(&deployCmdOptions.imageTag, "image-tag", "t", "", "image tag to update the container images to")

	// Configure required flags, applying to this specific command.
	deployCmd.MarkFlagRequired("image-tag")
}

func (options *deployOptions) validate() {
	if options.imageTag == "" {
		log.Fatal("image-tag flag must be set and should not be blank")
	}
}

func (options *deployOptions) run() {
	awsLogger := logging.LoggerFunc(func(classification logging.Classification, format string, v ...interface{}) {
		switch classification {
		case logging.Debug:
			log.Debug(format)
		case logging.Warn:
			log.Warn(format)
		}
	})

	awsCfg, err := config.LoadDefaultConfig(context.TODO(), config.WithLogger(awsLogger))
	if err != nil {
		log.Fatalf("unable to load aws config: %v", err)
	}
	client := ecs.NewFromConfig(awsCfg)

	toolConfig.DeployTasks(&options.imageTag, client)
	toolConfig.DeployServices(&options.imageTag, client)
}
