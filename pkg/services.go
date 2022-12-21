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
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"

	log "github.com/sirupsen/logrus"
)

func (config *Config) DeployServices(newContainerImageTag *string, client *ecs.Client) error {
	clusterSublogger := log.WithFields(log.Fields{"cluster": config.Cluster})
	clusterSublogger.Info("starting rollout to services")

	// Get list of services to update from the config file but do not proceed if
	// there are no services to update.
	numberOfServices := len(config.Services)
	if numberOfServices == 0 {
		clusterSublogger.Warn("skipping rollout to services, none found")

		return nil
	}

	// Process each service on its own asynchronously to reduce the amount of
	// time spent rolling them out. We should update each service at the same
	// time.
	serviceDeployErrors := make(chan error, numberOfServices)
	wg := sync.WaitGroup{}
	wg.Add(numberOfServices)
	for index := range config.Services {
		go func(serviceConfig *Service) {
			defer wg.Done()

			err := deployService(&config.Cluster, serviceConfig, newContainerImageTag, client, clusterSublogger)
			if err != nil {
				serviceDeployErrors <- err
			}
		}(&config.Services[index])
	}
	wg.Wait()
	close(serviceDeployErrors)

	failedCount := len(serviceDeployErrors)
	completedCount := numberOfServices - len(serviceDeployErrors)
	clusterSublogger.Infof("services report - total: %d, successful: %d, failed: %d", numberOfServices, completedCount, failedCount)

	if failedCount > 0 {
		err := fmt.Errorf("unable to deploy all services")

		return err
	}

	clusterSublogger.Info("completed rollout to services")

	return nil
}

func deployService(cluster *string, serviceConfig *Service, newContainerImageTag *string, client *ecs.Client, logger *log.Entry) error {
	// Set up new logger with the service name.
	serviceSublogger := logger.WithField("service", serviceConfig.Name)

	// Fetch full profile of the service so that later we can reference its
	// attributes i.e. task definitions.
	serviceSublogger.Debug("fetching service profile")
	serviceParams := &ecs.DescribeServicesInput{
		Cluster:  cluster,
		Services: []string{serviceConfig.Name},
	}
	serviceResult, err := client.DescribeServices(context.TODO(), serviceParams)
	if err != nil {
		serviceSublogger.Errorf("unable to fetch service profile: %v", err)

		return err
	}

	// If the service is not found then stop deploying to the service. We should
	// also only ever receive one service.
	if len(serviceResult.Services) == 0 {
		err = errors.New("skipping deploy, service not found")
		serviceSublogger.Error(err)

		return err
	}
	service := serviceResult.Services[0]

	// Store information on which containers should be updated.
	taskContainerUpdateable := make(map[string]bool)
	for _, containerName := range serviceConfig.Containers {
		taskContainerUpdateable[containerName] = true
	}

	// Generate new task definition with the required changes.
	taskDefinitionInput := GenerateTaskDefinitionInput{
		ImageTag:             newContainerImageTag,
		TaskDefinition:       service.TaskDefinition,
		UpdateableContainers: taskContainerUpdateable,
	}
	newTaskDefinition, taskDefinitionUpdated, err := GenerateTaskDefinition(&taskDefinitionInput, client, serviceSublogger)
	if err != nil {
		serviceSublogger.Errorf("error generating task definition")

		return err
	}

	// Prepare parameters for service.
	updateServiceParams := &ecs.UpdateServiceInput{
		Service:                       service.ServiceName,
		CapacityProviderStrategy:      service.CapacityProviderStrategy,
		Cluster:                       service.ClusterArn,
		DeploymentConfiguration:       service.DeploymentConfiguration,
		DesiredCount:                  &service.DesiredCount,
		EnableECSManagedTags:          &service.EnableECSManagedTags,
		EnableExecuteCommand:          &service.EnableECSManagedTags,
		HealthCheckGracePeriodSeconds: service.HealthCheckGracePeriodSeconds,
		LoadBalancers:                 service.LoadBalancers,
		NetworkConfiguration:          service.NetworkConfiguration,
		PlacementConstraints:          service.PlacementConstraints,
		PlacementStrategy:             service.PlacementStrategy,
		PlatformVersion:               service.PlatformVersion,
		PropagateTags:                 service.PropagateTags,
		ServiceRegistries:             service.ServiceRegistries,
	}

	// Set force.
	if serviceConfig.Force != nil {
		serviceSublogger.Debug("setting forced deploy")

		updateServiceParams.ForceNewDeployment = *serviceConfig.Force
	}

	// Set maximum wait time.
	maxWaitTime := 15 * time.Minute
	if serviceConfig.MaxWait != nil {
		serviceSublogger.Debug("setting maximum wait time")

		maxWaitTime = time.Duration(*serviceConfig.MaxWait) * time.Minute
	}

	// Set task definition.
	if taskDefinitionUpdated {
		serviceSublogger.Info("updated task definition, using new one")
		updateServiceParams.TaskDefinition = newTaskDefinition.TaskDefinitionArn
	} else {
		serviceSublogger.Info("no changes to previous task definition, using latest")
		updateServiceParams.TaskDefinition = &serviceConfig.Name
	}

	// Update service to reflect changes.
	serviceSublogger.Debug("attempting to update service")
	_, err = client.UpdateService(context.TODO(), updateServiceParams)
	if err != nil {
		serviceSublogger.Errorf("unable to update service: %v", err)

		return err
	}
	serviceSublogger.Info("updated service successfully")

	// Watch service deployment until all have a final status.
	serviceSublogger.Info("watch service rollout progress")
	watchService(cluster, &service, client, serviceSublogger)

	// Make sure we wait for the service to be stable.
	serviceSublogger.Info("checking if service is stable")
	waiter := ecs.NewServicesStableWaiter(client)
	err = waiter.Wait(context.TODO(), serviceParams, maxWaitTime, func(o *ecs.ServicesStableWaiterOptions) {
		o.MinDelay = 5 * time.Second
		o.MaxDelay = 120 * time.Second
		o.LogWaitAttempts = log.IsLevelEnabled(log.DebugLevel) || log.IsLevelEnabled(log.TraceLevel)
	})
	if err != nil {
		serviceSublogger.Errorf("unable to check if service is stable: %v", err)

		return err

	}

	serviceSublogger.Info("service is stable")

	return nil
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

		// PRIMARY - the most recent deployment of a service. ACTIVE - a service
		// deployment that still has running tasks, but are in the process of
		// being replaced with a new PRIMARY deployment. INACTIVE - A deployment
		// that has been completely replaced.
		hasCompletedPrimary := false
		hasActiveDeployment := false
		for _, deployment := range service.Deployments {
			// Set up logger with the deployment identifier.
			deploymentSublogger := serviceSublogger.WithField("deployment-id", *deployment.Id)
			deploymentSublogger.Infof("watching ... service: %s, deployment: %s, rollout: %d/%d (%d pending)", strings.ToLower(*service.Status), strings.ToLower(*deployment.Status), deployment.RunningCount, deployment.DesiredCount, deployment.PendingCount)

			if (*deployment.Status == "PRIMARY") && (deployment.RolloutState == types.DeploymentRolloutStateCompleted) {
				hasCompletedPrimary = true
			}

			if *deployment.Status == "ACTIVE" {
				hasActiveDeployment = true
			}
		}

		// A service has an ACTIVE deployment if it is still being rolled out.
		// but if the service's PRIMARY is in a completed state and it doesn't
		// have an ACTIVE deployment then the rollout is done and there's no
		// need to watch it any longer.
		if hasCompletedPrimary && !hasActiveDeployment {
			serviceSublogger.Debugf("primary deployment completed, no active deployment")

			break
		}

		<-ticker
	}
}
