package pkg

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"

	log "github.com/sirupsen/logrus"
)

func (config *Config) DeployTasks(newContainerImageTag *string, client *ecs.Client) {
	clusterSublogger := log.WithFields(log.Fields{
		"cluster": *config.Cluster,
	})
	clusterSublogger.Info("starting rollout to tasks")

	// Get list of tasks to update from the config file but do not proceed if
	// there are no tasks to update.
	if len(config.Tasks) == 0 {
		clusterSublogger.Warn("skipping rollout to tasks, none found")

		return
	}

	// Process each task on its own asynchronously. The idea is that tasks are
	// short-lived deployment steps that are pre-requisites to the deployment.
	// It's worth noting that all tasks must complete before the deployment
	// starts.
	wg := sync.WaitGroup{}
	wg.Add(len(config.Tasks))
	for _, taskConfig := range config.Tasks {
		go deployTask(config.Cluster, taskConfig, newContainerImageTag, client, &wg)
	}
	wg.Wait()

	clusterSublogger.Info("completed rollout to tasks")
}

func deployTask(cluster *string, taskConfig *Task, newContainerImageTag *string, client *ecs.Client, deployWg *sync.WaitGroup) {
	defer deployWg.Done()

	taskSublogger := log.WithFields(log.Fields{
		"cluster": *cluster,
		"task":    *taskConfig.Family,
	})

	// Store information on which containers should be updated.
	taskContainerUpdateable := make(map[string]bool)
	for _, containerName := range taskConfig.Containers {
		taskContainerUpdateable[*containerName] = true
	}

	// Generate new task definition with the required changes.
	taskDefinitionInput := GenerateTaskDefinitionInput{
		ImageTag:             newContainerImageTag,
		TaskDefinition:       taskConfig.Family,
		UpdateableContainers: taskContainerUpdateable,
	}
	newTaskDefinition, taskDefinitionUpdated := GenerateTaskDefinition(&taskDefinitionInput, client, taskSublogger)

	// Prepare parameters for task.
	taskSublogger.Info("preparing running task parameters")
	runTaskParams := &ecs.RunTaskInput{
		Cluster:              cluster,
		Count:                taskConfig.Count,
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
		runTaskParams.TaskDefinition = taskConfig.Family
	}

	// Set capacity provider strategies.
	if taskConfig.CapacityProviderStrategies != nil {
		taskSublogger.Debug("setting capacity provider strategies")
		capacityProviders := []types.CapacityProviderStrategyItem{}
		for _, capacityProviderStrategy := range taskConfig.CapacityProviderStrategies {
			capacityProviders = append(capacityProviders, types.CapacityProviderStrategyItem{
				CapacityProvider: capacityProviderStrategy.CapacityProvider,
				Base:             *capacityProviderStrategy.Base,
				Weight:           *capacityProviderStrategy.Weight,
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

		assignPublicIp := types.AssignPublicIpDisabled
		if *taskConfig.NetworkConfiguration.VpcConfiguration.AssignPublicIp {
			assignPublicIp = types.AssignPublicIpEnabled
		}

		securityGroups := []string{}
		for _, securityGroup := range taskConfig.NetworkConfiguration.VpcConfiguration.SecurityGroups {
			securityGroups = append(securityGroups, *securityGroup)
		}

		subnets := []string{}
		for _, subnet := range taskConfig.NetworkConfiguration.VpcConfiguration.Subnets {
			subnets = append(subnets, *subnet)
		}

		networkConfiguration := &types.NetworkConfiguration{
			AwsvpcConfiguration: &types.AwsVpcConfiguration{
				Subnets:        subnets,
				AssignPublicIp: assignPublicIp,
				SecurityGroups: securityGroups,
			},
		}

		runTaskParams.NetworkConfiguration = networkConfiguration
	}

	// Starts task(s) using the specified parameters.
	taskSublogger.Infof("attempting to run new task, desired count: %d", *taskConfig.Count)
	runTaskResult, err := client.RunTask(context.TODO(), runTaskParams)
	if err != nil {
		taskSublogger.Fatalf("unable to run new task, desired count: %d: %v", *taskConfig.Count, err)
	}
	taskSublogger.Infof("running new task, desired count: %d", *taskConfig.Count)

	// Watch each task on its own asynchronously. The number of tasks depends on
	// the count that was set. All tasks should be watched.
	wg := sync.WaitGroup{}
	wg.Add(len(runTaskResult.Tasks))
	for index, task := range runTaskResult.Tasks {
		taskNo := index + 1
		go watchTask(cluster, &taskNo, &task, client, taskSublogger, &wg)
	}
	wg.Wait()
}

func watchTask(cluster *string, taskNo *int, task *types.Task, client *ecs.Client, logger *log.Entry, watchWg *sync.WaitGroup) {
	defer watchWg.Done()

	ticker := time.NewTicker(time.Second * 3).C

	for {
		taskParams := &ecs.DescribeTasksInput{
			Cluster: cluster,
			Tasks:   []string{*task.TaskArn},
		}
		taskResult, err := client.DescribeTasks(context.TODO(), taskParams)
		if err != nil {
			logger.Fatalf("unable to fetch task profile: %v", err)
		}

		// If the task is not found then stop watching the task. We should
		// also only ever receive one task.
		if len(taskResult.Tasks) != 1 {
			break
		}
		task := taskResult.Tasks[0]

		// Get task ID from ARN since it's not available.
		var resourceIdRegex = regexp.MustCompile(`[^:/]*$`)
		taskId := resourceIdRegex.FindString(*task.TaskArn)

		// Set up logger with the task identifier.
		taskLogger := logger.WithField("task-id", taskId)
		taskLogger.Infof("watching task [%d] ... last status: %s, desired status: %s, health: %s", *taskNo, strings.ToLower(*task.LastStatus), strings.ToLower(*task.DesiredStatus), strings.ToLower(string(task.HealthStatus)))

		// When a task is started it can pass through several states before it
		// finishes on its own or is stopped manually. The expectation here is
		// that the task naturally progress through from PENDING to RUNNING to
		// STOPPED.
		stoppedTask := false
		if *task.LastStatus == "STOPPED" {
			stoppedTask = true
		}

		// If the task has stopped then there's no need to watch it any longer.
		if stoppedTask {
			for _, container := range task.Containers {
				containerReason := "none"
				if container.Reason != nil {
					containerReason = strings.ToLower(*container.Reason)
				}

				taskLogger.Infof("stopped task [%d] container [%s] ... exit code: %d, reason: %s", *taskNo, *container.Name, *container.ExitCode, containerReason)
			}
			taskLogger.Infof("stopped task [%d] ... reason: %s", *taskNo, strings.ToLower(string(*task.StoppedReason)))

			break
		}

		<-ticker
	}
}
