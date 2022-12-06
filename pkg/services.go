package pkg

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"

	log "github.com/sirupsen/logrus"
)

func (config *Config) DeployServices(newContainerImageTag *string, client *ecs.Client) {
	clusterSublogger := log.WithFields(log.Fields{"cluster": *config.Cluster})
	clusterSublogger.Info("starting rollout to services")

	// Get list of services to update from the config file but do not proceed if
	// there are no services to update.
	if len(config.Services) == 0 {
		clusterSublogger.Warn("skipping rollout to services, none found")

		return
	}

	// Process each service on its own asynchronously to reduce the amount of
	// time spent rolling them out. We should update each service at the same
	// time.
	wg := sync.WaitGroup{}
	wg.Add(len(config.Services))
	for _, serviceConfig := range config.Services {
		go deployService(config.Cluster, serviceConfig, newContainerImageTag, client, clusterSublogger, &wg)
	}
	wg.Wait()

	clusterSublogger.Info("completed rollout to services")
}

func deployService(cluster *string, serviceConfig *Service, newContainerImageTag *string, client *ecs.Client, logger *log.Entry, deployWg *sync.WaitGroup) {
	defer deployWg.Done()

	// Set up new logger with the service name.
	serviceSublogger := logger.WithField("service", *serviceConfig.Name)

	// Fetch full profile of the service so that later we can reference its
	// attributes i.e. task definitions.
	serviceSublogger.Info("fetching service profile")
	serviceParams := &ecs.DescribeServicesInput{
		Cluster:  cluster,
		Services: []string{*serviceConfig.Name},
	}
	serviceResult, err := client.DescribeServices(context.TODO(), serviceParams)
	if err != nil {
		serviceSublogger.Errorf("unable to fetch service profile: %v", err)

		return
	}

	// If the service is not found then stop deploying to the service. We should
	// also only ever receive one service.
	if len(serviceResult.Services) == 0 {
		serviceSublogger.Error("skipping deploy, service not found")

		return
	}
	service := serviceResult.Services[0]

	// Store information on which containers should be updated.
	taskContainerUpdateable := make(map[string]bool)
	for _, containerName := range serviceConfig.Containers {
		taskContainerUpdateable[*containerName] = true
	}

	// Generate new task definition with the required changes.
	taskDefinitionInput := GenerateTaskDefinitionInput{
		ImageTag:             newContainerImageTag,
		TaskDefinition:       service.TaskDefinition,
		UpdateableContainers: taskContainerUpdateable,
	}
	newTaskDefinition, taskDefinitionUpdated := GenerateTaskDefinition(&taskDefinitionInput, client, serviceSublogger)

	// Prepare parameters for service.
	updateServiceParams := &ecs.UpdateServiceInput{
		Service:                       service.ServiceName,
		CapacityProviderStrategy:      service.CapacityProviderStrategy,
		Cluster:                       service.ClusterArn,
		DeploymentConfiguration:       service.DeploymentConfiguration,
		DesiredCount:                  &service.DesiredCount,
		EnableECSManagedTags:          &service.EnableECSManagedTags,
		EnableExecuteCommand:          &service.EnableECSManagedTags,
		ForceNewDeployment:            *serviceConfig.Force,
		HealthCheckGracePeriodSeconds: service.HealthCheckGracePeriodSeconds,
		LoadBalancers:                 service.LoadBalancers,
		NetworkConfiguration:          service.NetworkConfiguration,
		PlacementConstraints:          service.PlacementConstraints,
		PlacementStrategy:             service.PlacementStrategy,
		PlatformVersion:               service.PlatformVersion,
		PropagateTags:                 service.PropagateTags,
		ServiceRegistries:             service.ServiceRegistries,
	}

	// Set task definition.
	if taskDefinitionUpdated {
		serviceSublogger.Info("updated task definition, using new one")
		updateServiceParams.TaskDefinition = newTaskDefinition.TaskDefinitionArn
	} else {
		serviceSublogger.Info("no changes to previous task definition, using latest")
		updateServiceParams.TaskDefinition = serviceConfig.Name
	}

	// Update service to reflect changes.
	serviceSublogger.Info("attempting to update service")
	_, err = client.UpdateService(context.TODO(), updateServiceParams)
	if err != nil {
		serviceSublogger.Errorf("unable to update service: %v", err)

		return
	}
	serviceSublogger.Info("updated service successfully")

	// Watch each service deployment until all have a final status.
	serviceSublogger.Info("watch rollout progress of services")
	watchService(cluster, &service, client, serviceSublogger)

	// Make sure we wait for rollout of all services.
	serviceSublogger.Info("checking if all services are stable")
	waiter := ecs.NewServicesStableWaiter(client)
	maxWaitTime := 15 * time.Minute
	err = waiter.Wait(context.TODO(), serviceParams, maxWaitTime, func(o *ecs.ServicesStableWaiterOptions) {
		o.MinDelay = 5 * time.Second
		o.MaxDelay = 120 * time.Second
		o.LogWaitAttempts = log.IsLevelEnabled(log.DebugLevel) || log.IsLevelEnabled(log.TraceLevel)
	})
	if err != nil {
		serviceSublogger.Errorf("unable to check if all services are stable: %v", err)

		return

	}
}

func watchService(cluster *string, service *types.Service, client *ecs.Client, serviceSublogger *log.Entry) {
	ticker := time.NewTicker(time.Second * 3).C

	for {
		serviceParams := &ecs.DescribeServicesInput{
			Cluster:  cluster,
			Services: []string{*service.ServiceName},
		}
		serviceResult, err := client.DescribeServices(context.TODO(), serviceParams)
		if err != nil {
			serviceSublogger.Errorf("unable to fetch service profile: %v", err)

			break
		}

		// If the service is not found then stop watching the service. We should
		// also only ever receive one service anyway.
		if len(serviceResult.Services) == 0 {
			serviceSublogger.Error("stopped watching, service not found")

			break
		}
		service := serviceResult.Services[0]

		activeService := false
		for _, deployment := range service.Deployments {
			// Set up logger with the deployment identifier.
			deploymentSublogger := serviceSublogger.WithField("deployment-id", deployment.Id)
			deploymentSublogger.Infof("watching ... service: %s, deployment: %s, rollout: %d/%d (%d pending)", strings.ToLower(*service.Status), strings.ToLower(*deployment.Status), deployment.RunningCount, deployment.DesiredCount, deployment.PendingCount)

			// PRIMARY The most recent deployment of a service. ACTIVE A service
			// deployment that still has running tasks, but are in the process
			// of being replaced with a new PRIMARY deployment. INACTIVE A
			// deployment that has been completely replaced.
			//
			// If a service has an ACTIVE deployment then that means that it's
			// still being rolled out.
			if *deployment.Status == "ACTIVE" {
				activeService = true
			}
		}

		// If the service is active then there's no need to watch it any
		// longer.
		if activeService {
			serviceSublogger.Infof("service is active")

			break
		}

		<-ticker
	}
}
