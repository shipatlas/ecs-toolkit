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
	"runtime/debug"
	"time"

	"github.com/shipatlas/ecs-toolkit/utils"
	"github.com/spf13/cobra"
)

type versionOptions struct {
	short bool
}

var (
	versionCmdLong = utils.LongDesc(`
		Print out all version information i.e. name, tag, revision, build time etc.`)

	versionCmdExamples = utils.Examples(`
		# Print out version information
		ecs-toolkit version
		
		# Print out just the version tag
		ecs-toolkit version --short`)

	versionCmdOptions     = &versionOptions{}
	versionSourceModified bool
	versionSourceRevision string
	versionSourceTime     time.Time
	versionTag            string
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:     "version",
	Short:   "Print out version information",
	Long:    versionCmdLong,
	Example: versionCmdExamples,
	Args: func(cmd *cobra.Command, args []string) error {
		err := cobra.NoArgs(cmd, args)

		return err
	},
	Run: func(cmd *cobra.Command, args []string) {
		versionCmdOptions.complete()
		versionCmdOptions.run()
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)

	// Local flags, which, will be global for the application.
	versionCmd.Flags().BoolVar(&versionCmdOptions.short, "short", false, "print version tag only")
}

func (options *versionOptions) complete() {
	info, _ := debug.ReadBuildInfo()

	for _, setting := range info.Settings {
		if setting.Key == "vcs.modified" && setting.Value == "true" {
			versionSourceModified = true
		}

		if setting.Key == "vcs.revision" {
			versionSourceRevision = setting.Value
		}

		if setting.Key == "vcs.time" {
			versionSourceTime, _ = time.Parse(time.RFC3339, setting.Value)
		}
	}

	if versionSourceModified {
		versionSourceRevision = fmt.Sprintf("%s (modified)", versionSourceRevision)
	}

	if versionTag == "" {
		versionTag = "(development)"
	}
}

func (options *versionOptions) run() {
	if options.short {
		fmt.Println(versionTag)

		return
	}

	fmt.Printf("Name:      %s\n", "ECS Toolkit")
	fmt.Printf("Version:   %s\n", versionTag)
	fmt.Printf("Revision:  %s\n", versionSourceRevision)
	fmt.Printf("Time:      %s\n", versionSourceTime)
}
