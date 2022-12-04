package pkg

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"

	dockerparser "github.com/novln/docker-parser"
	log "github.com/sirupsen/logrus"
)

func (config *Config) DeployTasks(newContainerImageTag *string, client *ecs.Client) {
	clusterSublogger := log.WithFields(log.Fields{
		"cluster": *config.Cluster,
	})
	clusterSublogger.Info("starting rollout to tasks")

	// Get list of tasks to update from the config file but do not proceed if
	// there are no tasks to update.
	configTaskFamilyNames := config.TaskFamilyNames()
	if len(configTaskFamilyNames) == 0 {
		clusterSublogger.Warn("skipping rollout to tasks, none found")

		return
	}

	for _, configTaskFamilyName := range configTaskFamilyNames {
		config.deployTask(&configTaskFamilyName, newContainerImageTag, client)
	}

	log.Fatal("--- STOP ---")

	clusterSublogger.Info("completed rollout to tasks")
}

func (config *Config) deployTask(taskFamilyName *string, newContainerImageTag *string, client *ecs.Client) {
	taskSublogger := log.WithFields(log.Fields{
		"cluster": *config.Cluster,
		"task":    *taskFamilyName,
	})

	// Prepare task mapping for easy lookup later, basically create a map with
	// the task family as the key and the task as the value in the tasks list as
	// the value.
	taskMapping := make(map[string]Task)
	for _, task := range config.Tasks {
		taskMapping[*task.Family] = *task
	}

	// Fetch full profile of the latest task definition.
	taskSublogger.Info("fetching task definition profile")
	taskDefinitionParams := &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: taskFamilyName,
		Include: []types.TaskDefinitionField{
			types.TaskDefinitionFieldTags,
		},
	}
	taskDefinitionResult, err := client.DescribeTaskDefinition(context.TODO(), taskDefinitionParams)
	if err != nil {
		taskSublogger.Fatalf("unable to fetch task definition profile: %v", err)
	}

	// Copy details of the current task definition to use a foundation of a
	// new revision.
	taskSublogger.Infof("building new task definition from %s:%d", *taskDefinitionResult.TaskDefinition.Family, taskDefinitionResult.TaskDefinition.Revision)
	registerTaskDefinitionParams := &ecs.RegisterTaskDefinitionInput{
		ContainerDefinitions:    taskDefinitionResult.TaskDefinition.ContainerDefinitions,
		Family:                  taskDefinitionResult.TaskDefinition.Family,
		Cpu:                     taskDefinitionResult.TaskDefinition.Cpu,
		EphemeralStorage:        taskDefinitionResult.TaskDefinition.EphemeralStorage,
		ExecutionRoleArn:        taskDefinitionResult.TaskDefinition.ExecutionRoleArn,
		InferenceAccelerators:   taskDefinitionResult.TaskDefinition.InferenceAccelerators,
		IpcMode:                 taskDefinitionResult.TaskDefinition.IpcMode,
		Memory:                  taskDefinitionResult.TaskDefinition.Memory,
		NetworkMode:             taskDefinitionResult.TaskDefinition.NetworkMode,
		PidMode:                 taskDefinitionResult.TaskDefinition.PidMode,
		PlacementConstraints:    taskDefinitionResult.TaskDefinition.PlacementConstraints,
		ProxyConfiguration:      taskDefinitionResult.TaskDefinition.ProxyConfiguration,
		RequiresCompatibilities: taskDefinitionResult.TaskDefinition.RequiresCompatibilities,
		RuntimePlatform:         taskDefinitionResult.TaskDefinition.RuntimePlatform,
		TaskRoleArn:             taskDefinitionResult.TaskDefinition.TaskRoleArn,
		Volumes:                 taskDefinitionResult.TaskDefinition.Volumes,
	}

	// Copy tags only if they exist else it will error out if you pass in an
	// empty list of tags.
	if len(taskDefinitionResult.Tags) >= 1 {
		registerTaskDefinitionParams.Tags = taskDefinitionResult.Tags
	}

	// Prepare task container mapping for easy lookup of containers that
	// should be updated, basically create a map with the container name as
	// the key and `true` as the value. If container lookup is found, then
	// that's an indicator of presence which can be used as a check.
	taskContainerUpdateable := make(map[string]bool)
	for _, containerName := range taskMapping[*taskFamilyName].Containers {
		taskContainerUpdateable[*containerName] = true
	}

	// For the new revision of the task definition update the image tag of
	// each container (where applicable).
	for i, containerDefinition := range registerTaskDefinitionParams.ContainerDefinitions {
		containerName := *containerDefinition.Name
		containerSublogger := log.WithFields(log.Fields{
			"cluster":   *config.Cluster,
			"task":      *taskFamilyName,
			"container": containerName,
		})

		// Only proceed to update container image tag if the container is on
		// the list of containers to update.
		if !taskContainerUpdateable[containerName] {
			containerSublogger.Warn("skipping container image tag update, not on the container list")

			continue
		}

		oldContainerImage := *containerDefinition.Image
		parsedImage, err := dockerparser.Parse(oldContainerImage)
		if err != nil {
			containerSublogger.Fatalf("unable to parse current container image %s: %v", oldContainerImage, err)
		}
		oldContainerImageTag := parsedImage.Tag()
		newContainerImage := strings.Replace(oldContainerImage, oldContainerImageTag, *newContainerImageTag, 1)
		containerSublogger.Debugf("container image registry: %s", parsedImage.Registry())
		containerSublogger.Debugf("container image name: %s", parsedImage.ShortName())
		containerSublogger.Infof("old container image tag: %s", oldContainerImageTag)
		containerSublogger.Infof("new container image tag: %s", *newContainerImageTag)

		// If the old and new image tags are the same then there's no need
		// to update the image and consequently the task definition.
		if oldContainerImageTag == *newContainerImageTag {
			containerSublogger.Warn("skipping container image tag update, no changes")

			continue
		}

		*registerTaskDefinitionParams.ContainerDefinitions[i].Image = newContainerImage
	}

	// Register a new updated version of the task definition i.e. with new
	// container image tags.
	taskSublogger.Info("registering new task definition")
	registerTaskDefinitionResult, err := client.RegisterTaskDefinition(context.TODO(), registerTaskDefinitionParams)
	if err != nil {
		taskSublogger.Fatalf("unable to register new task definition: %v", err)
	}
	newTaskDefinition := fmt.Sprintf("%s:%d", *registerTaskDefinitionResult.TaskDefinition.Family, registerTaskDefinitionResult.TaskDefinition.Revision)
	taskSublogger.Infof("successfully registered new task definition %s", newTaskDefinition)

	task := taskMapping[*taskFamilyName]

	// Prepare task definition.
	taskSublogger.Info("building new task configuration")
	runTaskParams := &ecs.RunTaskInput{
		TaskDefinition:       registerTaskDefinitionResult.TaskDefinition.TaskDefinitionArn,
		Cluster:              config.Cluster,
		Count:                task.Count,
		EnableECSManagedTags: true,
		EnableExecuteCommand: false,
		PropagateTags:        types.PropagateTagsTaskDefinition,
	}

	// Set capacity provider strategies
	if task.CapacityProviderStrategies != nil {
		taskSublogger.Debug("setting capacity provider strategies")
		capacityProviders := []types.CapacityProviderStrategyItem{}
		for _, capacityProviderStrategy := range task.CapacityProviderStrategies {
			capacityProviders = append(capacityProviders, types.CapacityProviderStrategyItem{
				CapacityProvider: capacityProviderStrategy.CapacityProvider,
				Base:             *capacityProviderStrategy.Base,
				Weight:           *capacityProviderStrategy.Weight,
			})
		}

		runTaskParams.CapacityProviderStrategy = capacityProviders
	}

	// Set launch type
	if task.LaunchType != nil {
		taskSublogger.Debugf("setting launch type to %s", *task.LaunchType)
		switch *task.LaunchType {
		case "ec2":
			runTaskParams.LaunchType = types.LaunchTypeEc2
		case "fargate":
			runTaskParams.LaunchType = types.LaunchTypeFargate
		case "external":
			runTaskParams.LaunchType = types.LaunchTypeExternal
		}
	}

	// Set network configuration
	if task.NetworkConfiguration != nil {
		taskSublogger.Debug("setting network configuration")

		assignPublicIp := types.AssignPublicIpDisabled
		if *task.NetworkConfiguration.VpcConfiguration.AssignPublicIp {
			assignPublicIp = types.AssignPublicIpEnabled
		}

		securityGroups := []string{}
		for _, securityGroup := range task.NetworkConfiguration.VpcConfiguration.SecurityGroups {
			securityGroups = append(securityGroups, *securityGroup)
		}

		subnets := []string{}
		for _, subnet := range task.NetworkConfiguration.VpcConfiguration.Subnets {
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
