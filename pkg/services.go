package pkg

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ecs"

	log "github.com/sirupsen/logrus"
)

func (config *Config) DeployServices(newContainerImageTag *string, client *ecs.Client) {
	clusterSublogger := log.WithFields(log.Fields{
		"cluster": *config.Cluster,
	})
	clusterSublogger.Info("starting rollout to services")

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
		clusterSublogger.Warn("skipping rollout to services, none found")

		return
	}

	// Fetch full profiles of the services so that later we can reference their
	// attributes e.g. their task definitions.
	clusterSublogger.Infof("fetching service profiles: %s", strings.Join(configServiceNames, ", "))
	servicesParams := &ecs.DescribeServicesInput{
		Cluster:  config.Cluster,
		Services: configServiceNames,
	}
	servicesResult, err := client.DescribeServices(context.TODO(), servicesParams)
	if err != nil {
		clusterSublogger.Fatalf("unable to fetch service profiles: %v", err)
	}

	// It's not guaranteed that all the services listed exist on the cluster so
	// generate a new list of service names of the ones that do.
	clusterServiceNames := []string{}
	for _, service := range servicesResult.Services {
		clusterServiceNames = append(clusterServiceNames, *service.ServiceName)
	}

	// If there are no services in the cluster then we should bail out.
	if len(clusterServiceNames) == 0 {
		clusterSublogger.Fatal("unable to proceed with deployment, services not found in the cluster")
	}

	// Raise warning if there's a mismatch in the services in the config and
	// those found in the cluster. Make sure we update DescribeServicesInput
	// just in case it's ever used further down.
	if len(configServiceNames) > len(clusterServiceNames) {
		servicesParams.Services = clusterServiceNames
		clusterSublogger.Warnf("some services missing, limiting to: %s", strings.Join(clusterServiceNames, ", "))
	}

	// Loop through all services, fetch the latest task definition, make a new
	// revision of it with updated image tags and finally update the service to
	// use the new revision.
	updatedServiceNames := []*string{}
	for _, service := range servicesResult.Services {
		serviceSublogger := log.WithFields(log.Fields{
			"cluster": *config.Cluster,
			"service": *service.ServiceName,
		})

		// Store information on which containers should be updated.
		serviceContainerUpdateable := make(map[string]bool)
		for _, containerName := range serviceMapping[*service.ServiceName].Containers {
			serviceContainerUpdateable[*containerName] = true
		}

		// Generate new task definition with the required changes.
		taskDefinitionInput := GenerateTaskDefinitionInput{
			ImageTag:             newContainerImageTag,
			TaskDefinition:       service.TaskDefinition,
			UpdateableContainers: serviceContainerUpdateable,
		}
		newTaskDefinition, taskDefinitionUpdated := GenerateTaskDefinition(&taskDefinitionInput, client, serviceSublogger)

		// Prepare parameters for task
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
			TaskDefinition:                newTaskDefinition.TaskDefinitionArn,
		}

		// Set task definition
		if taskDefinitionUpdated {
			serviceSublogger.Info("changes made, using new task definition")
			updateServiceParams.TaskDefinition = newTaskDefinition.TaskDefinitionArn
		} else {
			serviceSublogger.Info("no changes, using latest task definition")
			updateServiceParams.TaskDefinition = service.TaskDefinition
		}

		serviceSublogger.Info("attempting to update service")
		_, err = client.UpdateService(context.TODO(), updateServiceParams)
		if err != nil {
			clusterSublogger.Fatalf("unable to update service: %v", err)
		}

		updatedServiceNames = append(updatedServiceNames, service.ServiceName)
		serviceSublogger.Info("successfully updated service")
	}

	if len(updatedServiceNames) == 0 {
		clusterSublogger.Info("completed rollout to services")

		return
	}

	// Follow rollout of all services until all have a final status.
	clusterSublogger.Info("watch rollout progress of services")
	WatchServicesDeployment(config.Cluster, updatedServiceNames, client)

	// Make sure we wait for rollout of all services.
	clusterSublogger.Info("checking if all services are stable")
	waiter := ecs.NewServicesStableWaiter(client)
	maxWaitTime := 15 * time.Minute
	err = waiter.Wait(context.TODO(), servicesParams, maxWaitTime, func(o *ecs.ServicesStableWaiterOptions) {
		o.MinDelay = 5 * time.Second
		o.MaxDelay = 120 * time.Second
		o.LogWaitAttempts = log.IsLevelEnabled(log.DebugLevel) || log.IsLevelEnabled(log.TraceLevel)
	})
	if err != nil {
		clusterSublogger.Fatalf("unable to check if all services are stable: %v", err)

	}
	clusterSublogger.Info("completed rollout to services")
}

func WatchServicesDeployment(cluster *string, serviceNames []*string, client *ecs.Client) {
	wg := sync.WaitGroup{}
	wg.Add(len(serviceNames))
	for _, serviceName := range serviceNames {
		go watchServiceDeployment(cluster, serviceName, client, &wg)
	}
	wg.Wait()
}

func watchServiceDeployment(cluster *string, serviceName *string, client *ecs.Client, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(time.Second * 3).C

	for {
		serviceParams := &ecs.DescribeServicesInput{
			Cluster:  cluster,
			Services: []string{*serviceName},
		}
		serviceResult, err := client.DescribeServices(context.TODO(), serviceParams)
		if err != nil {
			log.WithFields(log.Fields{
				"cluster": *cluster,
			}).Fatalf("unable to fetch service profiles: %v", err)
		}

		// If the service is not found then stop watching the service. We should
		// also only ever receive one service.
		if len(serviceResult.Services) != 1 {
			break
		}

		activeService := false
		service := serviceResult.Services[0]
		for _, deployment := range service.Deployments {
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

			log.WithFields(log.Fields{
				"cluster": *cluster,
				"service": *serviceName,
			}).Infof("watching ... service: %s, deployment: %s, rollout: %d/%d (%d pending)", strings.ToLower(*service.Status), strings.ToLower(*deployment.Status), deployment.RunningCount, deployment.DesiredCount, deployment.PendingCount)
		}

		if !activeService {
			break
		}

		<-ticker
	}
}
