package cmd

import (
	"github.com/shipatlas/ecs-toolkit/utils"
	"github.com/spf13/cobra"
)

var (
	deployCmdLong = utils.LongDesc(`
		Deploy an application to AWS ECS`)
)

// deployCmd represents the deploy command
var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy an application to AWS ECS.",
	Long:  deployCmdLong,
	Run: func(cmd *cobra.Command, args []string) {
	},
}

func init() {
	rootCmd.AddCommand(deployCmd)
}
