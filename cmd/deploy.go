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
	"context"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/smithy-go/logging"
	"github.com/shipatlas/ecs-toolkit/pkg"
	"github.com/shipatlas/ecs-toolkit/utils"
	"github.com/spf13/cobra"

	log "github.com/sirupsen/logrus"
)

type deployOptions struct {
	imageTag      string
	skipTasks     bool
	skipTasksPre  bool
	skipTasksPost bool
}

var (
	deployCmdLong = utils.LongDesc(`
		Deploy an application to AWS ECS`)

	deployCmdExamples = utils.Examples(`
		# Deploy new revision of an application
		ecs-toolkit deploy --image-tag=5a853f72
		
		# Deploy new revision of an application but only update the services 
		# specified in the config, skips pre and post deployment tasks
		ecs-toolkit deploy --image-tag=5a853f72 --skip-tasks
		
		# Deploy new revision of an application but skip only pre-deployment tasks
		ecs-toolkit deploy --image-tag=5a853f72 --skip-pre-tasks
		
		# Deploy new revision of an application but skip only post-deployment tasks
		ecs-toolkit deploy --image-tag=5a853f72 --skip-post-tasks`)

	deployCmdOptions = &deployOptions{}
)

// deployCmd represents the deploy command
var deployCmd = &cobra.Command{
	Use:     "deploy",
	Short:   "Deploy an application to AWS ECS.",
	Long:    deployCmdLong,
	Example: deployCmdExamples,
	Args: func(cmd *cobra.Command, args []string) error {
		err := cobra.NoArgs(cmd, args)

		return err
	},
	Run: func(cmd *cobra.Command, args []string) {
		deployCmdOptions.validate()
		deployCmdOptions.run()
	},
}

func init() {
	rootCmd.AddCommand(deployCmd)

	// Local flags, which, will be global for the application.
	deployCmd.Flags().StringVarP(&deployCmdOptions.imageTag, "image-tag", "t", "", "image tag to update the container images to")
	deployCmd.Flags().BoolVar(&deployCmdOptions.skipTasks, "skip-tasks", false, "skips both pre-deployment & post-deployment tasks")
	deployCmd.Flags().BoolVar(&deployCmdOptions.skipTasksPre, "skip-pre-tasks", false, "skip only pre-deployment tasks")
	deployCmd.Flags().BoolVar(&deployCmdOptions.skipTasksPost, "skip-post-tasks", false, "skip only post-deployment tasks")

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

	if !options.skipTasks && !options.skipTasksPre {
		err = toolConfig.DeployTasks(&options.imageTag, pkg.TaskStagePre, client)
		if err != nil {
			log.Fatal("error deploying pre-deployment tasks, exiting!")
		}
	}

	err = toolConfig.DeployServices(&options.imageTag, client)
	if err != nil {
		log.Fatal("error deploying services, exiting!")
	}

	if !options.skipTasks && !options.skipTasksPost {
		err = toolConfig.DeployTasks(&options.imageTag, pkg.TaskStagePost, client)
		if err != nil {
			log.Fatal("error deploying post-deployment tasks, exiting!")
		}
	}
}
