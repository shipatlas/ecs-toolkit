package pkg

import (
	"context"
	"sync"

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

func deployTask(cluster *string, taskConfig *Task, newContainerImageTag *string, client *ecs.Client, wg *sync.WaitGroup) {
	defer wg.Done()

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
	taskSublogger.Info("building running task configuration")
	runTaskParams := &ecs.RunTaskInput{
		Cluster:              cluster,
		Count:                taskConfig.Count,
		EnableECSManagedTags: true,
		EnableExecuteCommand: false,
		PropagateTags:        types.PropagateTagsTaskDefinition,
	}

	// Set task definition.
	if taskDefinitionUpdated {
		taskSublogger.Info("changes made, using new task definition")
		runTaskParams.TaskDefinition = newTaskDefinition.TaskDefinitionArn
	} else {
		taskSublogger.Info("no changes, using latest task definition")
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

	// Starts a new task using the specified parameters.
	taskSublogger.Info("attempting to start new task")
	runTaskResult, err := client.RunTask(context.TODO(), runTaskParams)
	if err != nil {
		taskSublogger.Fatalf("unable to start new task: %v", err)
	}

	for i, newTask := range runTaskResult.Tasks {
		taskNo := i + 1
		taskSublogger.Infof("successfully started new task [%d] %s", taskNo, *newTask.TaskArn)
	}
}
