package cmd

import (
	"github.com/shipatlas/ecs-toolkit/utils"
	"github.com/spf13/cobra"
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
		log.Fatal().Msg("image-tag flag must be set and should not be blank")
	}
}
