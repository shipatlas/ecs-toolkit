package pkg

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	dockerparser "github.com/novln/docker-parser"
)

func (config *Config) DeployServices(newContainerImageTag *string, client *ecs.Client) {
	clusterSublogger := log.With().
		Str("cluster", *config.Cluster).
		Logger()
	clusterSublogger.Info().Msg("starting deployment of services")

	// Prepare service mapping for easy lookup later, basically create a map
	// with the service name as the key and the service as the value in the
	// services list as the value.
	serviceMapping := make(map[string]Service)
	for _, service := range config.Services {
		serviceMapping[*service.Name] = *service
	}

	// Get list of services to update from the config file but do not proceed if
	// there are no services to update.
	configServiceNames := config.ServiceNames()
	if len(configServiceNames) == 0 {
		clusterSublogger.Warn().Msg("skipping deployment of services, none found")

		return
	}

	// Fetch full profiles of the services so that later we can reference their
	// attributes e.g. their task definitions.
	clusterSublogger.Info().Msgf("fetching service profiles: %s", strings.Join(configServiceNames, ", "))
	servicesParams := &ecs.DescribeServicesInput{
		Cluster:  config.Cluster,
		Services: configServiceNames,
	}
	servicesResult, err := client.DescribeServices(context.TODO(), servicesParams)
	if err != nil {
		clusterSublogger.Fatal().Err(err).Msg("unable to fetch service profiles")
	}

	// It's not guaranteed that all the services listed exist on the cluster so
	// generate a new list of service names of the ones that do.
	clusterServiceNames := []string{}
	for _, service := range servicesResult.Services {
		clusterServiceNames = append(clusterServiceNames, *service.ServiceName)
	}

	// If there are no services in the cluster then we should bail out.
	if len(clusterServiceNames) == 0 {
		clusterSublogger.Fatal().Msg("unable to proceed with deployment, services not found in the cluster")
	}

	// Raise warning if there's a mismatch in the services in the config and
	// those found in the cluster. Make sure we update DescribeServicesInput
	// just in case it's ever used further down.
	if len(configServiceNames) > len(clusterServiceNames) {
		servicesParams.Services = clusterServiceNames
		clusterSublogger.Warn().Msgf("some services missing, limiting to: %s", strings.Join(clusterServiceNames, ", "))
	}

	// Loop through all services, fetch the latest task definition, make a new
	// revision of it with updated image tags and finally update the service to
	// use the new revision.
	for _, service := range servicesResult.Services {
		serviceSublogger := log.With().
			Str("cluster", *config.Cluster).
			Str("service", *service.ServiceName).
			Logger()

		// Fetch full profile of the latest task definition.
		serviceSublogger.Info().Msg("fetching service task definition profile")
		taskDefinitionParams := &ecs.DescribeTaskDefinitionInput{
			TaskDefinition: service.TaskDefinition,
			Include: []types.TaskDefinitionField{
				types.TaskDefinitionFieldTags,
			},
		}
		taskDefinitionResult, err := client.DescribeTaskDefinition(context.TODO(), taskDefinitionParams)
		if err != nil {
			serviceSublogger.Fatal().Err(err).Msg("unable to fetch service task definition profile")
		}

		// Copy details of the current task definition to use a foundation of a
		// new revision.
		serviceSublogger.Info().Msgf("building new task definition from %s:%d", *taskDefinitionResult.TaskDefinition.Family, taskDefinitionResult.TaskDefinition.Revision)
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

		// Prepare service container mapping for easy lookup of containers that
		// should be updated, basically create a map with the container name as
		// the key and `true` as the value. If container lookup is found, then
		// that's an indicator of presence which can be used as a check.
		serviceContainerUpdateable := make(map[string]bool)
		for _, containerName := range serviceMapping[*service.ServiceName].Containers {
			serviceContainerUpdateable[*containerName] = true
		}

		// For the new revision of the task definition update the image tag of
		// each container (where applicable).
		taskDefinitionUpdated := false
		for i, containerDefinition := range registerTaskDefinitionParams.ContainerDefinitions {
			containerName := *containerDefinition.Name
			containerSublogger := log.With().
				Str("cluster", *config.Cluster).
				Str("service", *service.ServiceName).
				Str("container", containerName).
				Logger()

			// Only proceed to update container image tag if the container is on
			// the list of containers to update.
			if !serviceContainerUpdateable[containerName] {
				containerSublogger.Warn().Msg("skipping container image tag update, not on the container list")

				continue
			}

			oldContainerImage := *containerDefinition.Image
			parsedImage, err := dockerparser.Parse(oldContainerImage)
			if err != nil {
				containerSublogger.Fatal().Err(err).Msgf("unable to parse current container image: %s", oldContainerImage)
			}
			oldContainerImageTag := parsedImage.Tag()
			newContainerImage := strings.Replace(oldContainerImage, oldContainerImageTag, *newContainerImageTag, 1)
			containerSublogger.Debug().Msgf("container image registry: %s", parsedImage.Registry())
			containerSublogger.Debug().Msgf("container image name: %s", parsedImage.ShortName())
			containerSublogger.Info().Msgf("old container image tag: %s", oldContainerImageTag)
			containerSublogger.Info().Msgf("new container image tag: %s", *newContainerImageTag)

			// If the old and new image tags are the same then there's no need
			// to update the image and consequently the task definition.
			if oldContainerImageTag == *newContainerImageTag {
				containerSublogger.Warn().Msg("skipping container image tag update, no changes")

				continue
			}

			*registerTaskDefinitionParams.ContainerDefinitions[i].Image = newContainerImage
			taskDefinitionUpdated = true
		}

		if !taskDefinitionUpdated {
			serviceSublogger.Warn().Msg("skipping registering new task definition, no changes")
			serviceSublogger.Warn().Msg("skipping service update, no changes")

			continue
		}

		// Register a new updated version of the task definition i.e. with new
		// container image tags.
		serviceSublogger.Info().Msg("registering new task definition")
		registerTaskDefinitionResult, err := client.RegisterTaskDefinition(context.TODO(), registerTaskDefinitionParams)
		if err != nil {
			serviceSublogger.Fatal().Err(err).Msg("unable to register new task definition")
		}
		newTaskDefinition := fmt.Sprintf("%s:%d", *registerTaskDefinitionResult.TaskDefinition.Family, registerTaskDefinitionResult.TaskDefinition.Revision)
		serviceSublogger.Info().Msgf("successfully registered new task definition %s", newTaskDefinition)

		// Update the service to use the new/latest revision of the task
		// definition.
		serviceSublogger.Info().Msg("update service to use new task definition")
		updateServiceParams := &ecs.UpdateServiceInput{
			Service:                       service.ServiceName,
			CapacityProviderStrategy:      service.CapacityProviderStrategy,
			Cluster:                       service.ClusterArn,
			DeploymentConfiguration:       service.DeploymentConfiguration,
			DesiredCount:                  &service.DesiredCount,
			EnableECSManagedTags:          &service.EnableECSManagedTags,
			EnableExecuteCommand:          &service.EnableECSManagedTags,
			ForceNewDeployment:            *serviceMapping[*service.ServiceName].Force,
			HealthCheckGracePeriodSeconds: service.HealthCheckGracePeriodSeconds,
			LoadBalancers:                 service.LoadBalancers,
			NetworkConfiguration:          service.NetworkConfiguration,
			PlacementConstraints:          service.PlacementConstraints,
			PlacementStrategy:             service.PlacementStrategy,
			PlatformVersion:               service.PlatformVersion,
			PropagateTags:                 service.PropagateTags,
			ServiceRegistries:             service.ServiceRegistries,
			TaskDefinition:                &newTaskDefinition,
		}
		_, err = client.UpdateService(context.TODO(), updateServiceParams)
		if err != nil {
			clusterSublogger.Fatal().Err(err).Msg("unable to update service to use new task definition")
		}
		serviceSublogger.Info().Msg("successfully updated service to use new task definition")
	}

	// Make sure we wait for rollout of all services
	clusterSublogger.Info().Msg("checking if all services are stable")
	waiter := ecs.NewServicesStableWaiter(client)
	maxWaitTime := 15 * time.Minute
	err = waiter.Wait(context.TODO(), servicesParams, maxWaitTime, func(o *ecs.ServicesStableWaiterOptions) {
		o.MinDelay = 5 * time.Second
		o.MaxDelay = 120 * time.Second
		o.LogWaitAttempts = (zerolog.GlobalLevel() == zerolog.DebugLevel) || (zerolog.GlobalLevel() == zerolog.TraceLevel)
	})
	if err != nil {
		clusterSublogger.Fatal().Err(err).Msg("unable to check if all services are stable")

	}
	clusterSublogger.Info().Msg("completed deployment of services")
}