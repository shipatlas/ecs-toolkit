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

package pkg

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"

	log "github.com/sirupsen/logrus"
)

func (config *Config) DeployTasks(newContainerImageTag *string, stage TaskStage, client *ecs.Client) error {
	clusterSublogger := log.WithFields(log.Fields{"cluster": config.Cluster})

	configTasks := []Task{}
	switch stage {
	case TaskStagePre:
		configTasks = config.Tasks.Pre
	case TaskStagePost:
		configTasks = config.Tasks.Post
	}

	// Get list of tasks to update from the config file but do not proceed if
	// there are no tasks to update.
	numberOfTasks := len(configTasks)
	if numberOfTasks == 0 {
		clusterSublogger.Warnf("skipping rollout of %s-deployment tasks, none found", stage)

		return nil
	}
	clusterSublogger.Infof("starting rollout of %s-deployment tasks", stage)

	// Process each service on in parallel to reduce the amount of time spent
	// rolling them out and evaluate the status to provide a summary report
	// after. Tasks are short-lived deployment steps that are pre-requisites to
	// the deployment. It's worth noting that all tasks must complete before the
	// deployment starts.
	var (
		failedCount  = 0
		skippedCount = 0
		wg           = sync.WaitGroup{}
	)
	for index := range configTasks {
		wg.Add(1)

		go func(taskConfig *Task) {
			defer wg.Done()

			status, err := deployTask(&config.Cluster, taskConfig, newContainerImageTag, client, clusterSublogger)
			if err != nil {
				if err != nil {
					switch status {
					case FailedStatus:
						failedCount = failedCount + 1
					case SkippedStatus:
						skippedCount = skippedCount + 1
					}
				}
			}
		}(&configTasks[index])
	}
	wg.Wait()

	successfulCount := numberOfTasks - (failedCount + skippedCount)
	clusterSublogger.Infof("tasks report - total: %d, successful: %d, skipped: %d, failed: %d", numberOfTasks, successfulCount, skippedCount, failedCount)

	if failedCount > 0 {
		err := fmt.Errorf("unable to deploy all %s-deployment tasks", stage)

		return err
	}

	clusterSublogger.Infof("completed rollout of %s-deployment tasks", stage)

	return nil
}

func deployTask(cluster *string, taskConfig *Task, newContainerImageTag *string, client *ecs.Client, logger *log.Entry) (Status, error) {
	// Set up new logger with the task family.
	taskSublogger := logger.WithField("task", taskConfig.Family)

	// Store information on which containers should be updated.
	taskContainerUpdateable := make(map[string]bool)
	for _, containerName := range taskConfig.Containers {
		taskContainerUpdateable[containerName] = true
	}

	// Generate new task definition with the required changes.
	taskDefinitionInput := GenerateTaskDefinitionInput{
		ImageTag:             newContainerImageTag,
		TaskDefinition:       &taskConfig.Family,
		UpdateableContainers: taskContainerUpdateable,
	}
	newTaskDefinition, taskDefinitionUpdated, err := GenerateTaskDefinition(&taskDefinitionInput, client, taskSublogger)
	if err != nil {
		taskSublogger.Errorf("error generating task definition")

		return FailedStatus, err
	}

	// Prepare parameters for task.
	taskSublogger.Info("preparing running task parameters")
	runTaskParams := &ecs.RunTaskInput{
		Cluster:              cluster,
		Count:                &taskConfig.Count,
		EnableECSManagedTags: true,
		EnableExecuteCommand: false,
		PropagateTags:        types.PropagateTagsTaskDefinition,
	}

	// Set task definition.
	if taskDefinitionUpdated {
		taskSublogger.Info("updated task definition, using new one")
		runTaskParams.TaskDefinition = newTaskDefinition.TaskDefinitionArn
	} else {
		taskSublogger.Info("no changes to previous task definition, using latest")
		runTaskParams.TaskDefinition = &taskConfig.Family
	}

	// Set capacity provider strategies.
	if len(taskConfig.CapacityProviderStrategies) > 0 {
		taskSublogger.Debug("setting capacity provider strategies")
		capacityProviders := []types.CapacityProviderStrategyItem{}
		for _, capacityProviderStrategy := range taskConfig.CapacityProviderStrategies {
			capacityProviders = append(capacityProviders, types.CapacityProviderStrategyItem{
				CapacityProvider: &capacityProviderStrategy.CapacityProvider,
				Base:             capacityProviderStrategy.Base,
				Weight:           capacityProviderStrategy.Weight,
			})
		}

		runTaskParams.CapacityProviderStrategy = capacityProviders
	}

	// Set launch type.
	if taskConfig.LaunchType != nil {
		taskSublogger.Debugf("setting launch type to %s", *taskConfig.LaunchType)
		switch *taskConfig.LaunchType {
		case "ec2":
			runTaskParams.LaunchType = types.LaunchTypeEc2
		case "fargate":
			runTaskParams.LaunchType = types.LaunchTypeFargate
		case "external":
			runTaskParams.LaunchType = types.LaunchTypeExternal
		}
	}

	// Set network configuration.
	if taskConfig.NetworkConfiguration != nil {
		taskSublogger.Debug("setting network configuration")

		assignPublicIP := types.AssignPublicIpDisabled
		if taskConfig.NetworkConfiguration.VpcConfiguration.AssignPublicIP {
			assignPublicIP = types.AssignPublicIpEnabled
		}

		networkConfiguration := &types.NetworkConfiguration{
			AwsvpcConfiguration: &types.AwsVpcConfiguration{
				AssignPublicIp: assignPublicIP,
				SecurityGroups: taskConfig.NetworkConfiguration.VpcConfiguration.SecurityGroups,
				Subnets:        taskConfig.NetworkConfiguration.VpcConfiguration.Subnets,
			},
		}

		runTaskParams.NetworkConfiguration = networkConfiguration
	}

	// Starts task(s) using the specified parameters.
	taskSublogger.Debugf("attempting to run new task, desired count: %d", taskConfig.Count)
	runTaskResult, err := client.RunTask(context.TODO(), runTaskParams)
	if err != nil {
		taskSublogger.Errorf("unable to run new task, desired count: %d: %v", taskConfig.Count, err)

		return FailedStatus, err
	}
	taskSublogger.Infof("running new task, desired count: %d", taskConfig.Count)

	// Watch each task on its own asynchronously. The number of tasks depends on
	// the count that was set. All tasks should be watched.
	numberOfTasks := len(runTaskResult.Tasks)
	taskWatchErrors := make(chan error, numberOfTasks)
	wg := sync.WaitGroup{}
	wg.Add(numberOfTasks)
	for index := range runTaskResult.Tasks {
		taskNo := index + 1

		go func(taskNo int, waitedOnTask types.Task) {
			defer wg.Done()

			err := watchTask(cluster, &taskNo, &waitedOnTask, client, taskSublogger)
			if err != nil {
				taskWatchErrors <- err
			}
		}(taskNo, runTaskResult.Tasks[index])
	}
	wg.Wait()
	close(taskWatchErrors)

	failedCount := len(taskWatchErrors)
	if failedCount > 0 {
		err := fmt.Errorf("unable to run all tasks")

		return FailedStatus, err
	}

	taskSublogger.Infof("tasks ran to completion, desired count: %d", taskConfig.Count)

	return SucceededStatus, nil
}

func watchTask(cluster *string, taskNo *int, task *types.Task, client *ecs.Client, logger *log.Entry) error {
	ticker := time.NewTicker(time.Second * 3).C

	for {
		taskParams := &ecs.DescribeTasksInput{
			Cluster: cluster,
			Tasks:   []string{*task.TaskArn},
		}
		taskResult, err := client.DescribeTasks(context.TODO(), taskParams)
		if err != nil {
			logger.Errorf("unable to fetch task profile: %v", err)

			return err
		}

		// If the task is not found or it has been deleted then stop watching
		// the task. We should also only ever receive one task.
		if len(taskResult.Tasks) == 0 {
			logger.Info("stopped watching, task not found")

			break
		}
		task := taskResult.Tasks[0]

		// Get task ID from ARN since it's not available.
		var resourceIDRegex = regexp.MustCompile(`[^:/]*$`)
		taskID := resourceIDRegex.FindString(*task.TaskArn)

		// Set up new logger with the task identifier.
		taskSublogger := logger.WithField("task-id", taskID)
		taskSublogger.Infof("watching task [%d] ... last status: %s, desired status: %s, health: %s", *taskNo, strings.ToLower(*task.LastStatus), strings.ToLower(*task.DesiredStatus), strings.ToLower(string(task.HealthStatus)))

		// When a task is started it can pass through several states before it
		// finishes on its own or is stopped manually. The expectation here is
		// that the task naturally progress through from PENDING to RUNNING to
		// STOPPED. If the task has stopped then there's no need to watch it any
		// longer.
		if *task.LastStatus == "STOPPED" {
			nonZeroExit := false
			for _, container := range task.Containers {
				var (
					containerExitCode = "none"
					containerName     = "unknown"
					containerReason   = "none"
				)

				if container.ExitCode != nil {
					containerExitCode = strconv.Itoa(int(*container.ExitCode))

					if *container.ExitCode != 0 {
						nonZeroExit = true
					}
				}

				if container.Name != nil {
					containerName = strings.ToLower(*container.Name)
				}

				if container.Reason != nil {
					containerReason = strings.ToLower(*container.Reason)
				}

				taskSublogger.Debugf("stopped task [%d] container [%s] ... exit code: %s, reason: %s", *taskNo, containerName, containerExitCode, containerReason)
			}

			exitMessage := fmt.Sprintf("stopped task [%d], reason: %s", *taskNo, strings.ToLower(string(*task.StoppedReason)))
			if nonZeroExit {
				err := fmt.Errorf("prematurely %s", exitMessage)
				taskSublogger.Error(err)

				return err
			}
			taskSublogger.Infof("successfully %s", exitMessage)

			break
		}

		<-ticker
	}

	return nil
}
