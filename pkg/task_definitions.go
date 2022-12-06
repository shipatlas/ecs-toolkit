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

type GenerateTaskDefinitionInput struct {
	// The docker image tag to use when updating the container image.
	//
	// This member is required.
	ImageTag *string

	// The task definition to use as a foundation for a new task definition.
	// Could be the family for the latest ACTIVE revision, family and revision
	// (family:revision) for a specific revision in the family, or full Amazon
	// Resource Name (ARN) of the task definition.
	//
	// This member is required.
	TaskDefinition *string

	// Container mapping for easy lookup of containers that should be updated,
	// basically create a map with the container name as the key and `true` as
	// the value.
	//
	// This member is required.
	UpdateableContainers map[string]bool
}

func GenerateTaskDefinition(input *GenerateTaskDefinitionInput, client *ecs.Client, logger *log.Entry) (*types.TaskDefinition, bool) {
	// Fetch full profile of the latest task definition.
	logger.Info("fetching task definition profile")
	taskDefinitionParams := &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: input.TaskDefinition,
		Include: []types.TaskDefinitionField{
			types.TaskDefinitionFieldTags,
		},
	}
	taskDefinitionResult, err := client.DescribeTaskDefinition(context.TODO(), taskDefinitionParams)
	if err != nil {
		logger.Fatalf("unable to fetch task definition profile: %v", err)
	}

	// Copy details of the task definition to use a foundation for the new
	// version of the task definition.
	logger.Infof("building new task definition from %s:%d", *taskDefinitionResult.TaskDefinition.Family, taskDefinitionResult.TaskDefinition.Revision)
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

	// For the new revision of the task definition update the image tag of
	// each container (where applicable).
	taskDefinitionUpdated := false
	for i, containerDefinition := range registerTaskDefinitionParams.ContainerDefinitions {
		containerName := *containerDefinition.Name
		containerSublogger := logger.WithField("container", containerName)

		// Only proceed to update container image tag if the container is on the
		// list of containers to update. If container name lookup in the map is
		// found and is true, then that's an indicator of updateability.
		if !input.UpdateableContainers[containerName] {
			containerSublogger.Warn("skipping container image tag update, not on the container list")

			continue
		}

		oldContainerImage := *containerDefinition.Image
		parsedImage, err := dockerparser.Parse(oldContainerImage)
		if err != nil {
			containerSublogger.Fatalf("unable to parse current container image %s: %v", oldContainerImage, err)
		}
		oldContainerImageTag := parsedImage.Tag()
		newContainerImageTag := input.ImageTag
		newContainerImage := strings.Replace(oldContainerImage, oldContainerImageTag, *newContainerImageTag, 1)

		// If the old and new image tags are the same then there's no need
		// to update the image and consequently the task definition.
		if oldContainerImageTag == *newContainerImageTag {
			containerSublogger.Warn("skipping container image tag update, no changes")

			continue
		}

		*registerTaskDefinitionParams.ContainerDefinitions[i].Image = newContainerImage
		taskDefinitionUpdated = true
		containerSublogger.Debugf("container image registry: %s", parsedImage.Registry())
		containerSublogger.Debugf("container image name: %s", parsedImage.ShortName())
		containerSublogger.Infof("old container image tag: %s", oldContainerImageTag)
		containerSublogger.Infof("new container image tag: %s", *newContainerImageTag)
	}

	// If task definition wasn't updated there's no need to update the service.
	if !taskDefinitionUpdated {
		logger.Warn("skipping registering new task definition, no changes")

		return nil, false
	}

	// Register a new updated version of the task definition i.e. with new
	// container image tags.
	logger.Info("registering new task definition")
	registerTaskDefinitionResult, err := client.RegisterTaskDefinition(context.TODO(), registerTaskDefinitionParams)
	if err != nil {
		logger.Fatalf("unable to register new task definition: %v", err)
	}
	newTaskDefinition := fmt.Sprintf("%s:%d", *registerTaskDefinitionResult.TaskDefinition.Family, registerTaskDefinitionResult.TaskDefinition.Revision)
	logger.Infof("successfully registered new task definition %s", newTaskDefinition)

	return registerTaskDefinitionResult.TaskDefinition, taskDefinitionUpdated
}
