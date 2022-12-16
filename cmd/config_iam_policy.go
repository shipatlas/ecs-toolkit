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
	"fmt"

	"github.com/shipatlas/ecs-toolkit/utils"
	"github.com/spf13/cobra"

	log "github.com/sirupsen/logrus"
)

type iamPolicyOptions struct {
	account string
	region  string
}

var (
	iamPolicyCmdLong = utils.LongDesc(`
		Generate IAM policy to attach to the IAM role or user.`)

	iamPolicyCmdExamples = utils.Examples(`
		# Print out generated IAM policy for account 123456789012
		ecs-toolkit config iam-policy -a 123456789012
		
		# Print out generated IAM policy for account 123456789012 and eu-west-1 region
		ecs-toolkit config iam-policy -a 123456789012 -r eu-west-1`)

	iamPolicyCmdAliases = []string{
		"policy",
	}

	iamPolicyCmdOptions = &iamPolicyOptions{}
)

// iamPolicyCmd represents the version command
var iamPolicyCmd = &cobra.Command{
	Use:     "iam-policy",
	Short:   "Generate IAM policy for use on AWS IAM",
	Long:    iamPolicyCmdLong,
	Aliases: iamPolicyCmdAliases,
	Example: iamPolicyCmdExamples,
	Args: func(cmd *cobra.Command, args []string) error {
		err := cobra.NoArgs(cmd, args)

		return err
	},
	Run: func(cmd *cobra.Command, args []string) {
		iamPolicyCmdOptions.validate()
		iamPolicyCmdOptions.run()
	},
}

func init() {
	configCmd.AddCommand(iamPolicyCmd)

	// Local flags, which, will be global for the application.
	iamPolicyCmd.Flags().StringVarP(&iamPolicyCmdOptions.account, "account", "a", "", "12-digit number that uniquely identifies an AWS account")
	iamPolicyCmd.Flags().StringVarP(&iamPolicyCmdOptions.region, "region", "r", "us-east-1", "separate geographic areas that AWS uses to house its infrastructure")

	// Configure required flags, applying to this specific command.
	iamPolicyCmd.MarkFlagRequired("account")
}

func (options *iamPolicyOptions) validate() {
	if options.account == "" {
		log.Fatal("account flag must be set and should not be blank")
	}

	if options.region == "" {
		log.Fatal("region flag must be set and should not be blank")
	}
}

func (options *iamPolicyOptions) run() {
	policy, err := toolConfig.GenerateIAMPolicy(options.account, options.region)
	if err != nil {
		log.Fatal("error generating IAM policy, exiting!")
	}

	fmt.Println(policy)
}
