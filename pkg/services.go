package pkg

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"

	dockerparser "github.com/novln/docker-parser"
	log "github.com/sirupsen/logrus"
)

func (config *Config) DeployServices(newContainerImageTag *string, client *ecs.Client) {
	clusterSublogger := log.WithFields(log.Fields{
		"cluster": *config.Cluster,
	})
	clusterSublogger.Info("starting deployment of services")

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
		clusterSublogger.Warn("skipping deployment of services, none found")

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

		// Fetch full profile of the latest task definition.
		serviceSublogger.Info("fetching service task definition profile")
		taskDefinitionParams := &ecs.DescribeTaskDefinitionInput{
			TaskDefinition: service.TaskDefinition,
			Include: []types.TaskDefinitionField{
				types.TaskDefinitionFieldTags,
			},
		}
		taskDefinitionResult, err := client.DescribeTaskDefinition(context.TODO(), taskDefinitionParams)
		if err != nil {
			serviceSublogger.Fatalf("unable to fetch service task definition profile: %v", err)
		}

		// Copy details of the current task definition to use a foundation of a
		// new revision.
		serviceSublogger.Infof("building new task definition from %s:%d", *taskDefinitionResult.TaskDefinition.Family, taskDefinitionResult.TaskDefinition.Revision)
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
			containerSublogger := log.WithFields(log.Fields{
				"cluster":   *config.Cluster,
				"service":   *service.ServiceName,
				"container": containerName,
			})

			// Only proceed to update container image tag if the container is on
			// the list of containers to update.
			if !serviceContainerUpdateable[containerName] {
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
			taskDefinitionUpdated = true
		}

		if !taskDefinitionUpdated {
			serviceSublogger.Warn("skipping registering new task definition, no changes")
			serviceSublogger.Warn("skipping service update, no changes")

			continue
		}

		// Register a new updated version of the task definition i.e. with new
		// container image tags.
		serviceSublogger.Info("registering new task definition")
		registerTaskDefinitionResult, err := client.RegisterTaskDefinition(context.TODO(), registerTaskDefinitionParams)
		if err != nil {
			serviceSublogger.Fatalf("unable to register new task definition: %v", err)
		}
		newTaskDefinition := fmt.Sprintf("%s:%d", *registerTaskDefinitionResult.TaskDefinition.Family, registerTaskDefinitionResult.TaskDefinition.Revision)
		serviceSublogger.Infof("successfully registered new task definition %s", newTaskDefinition)

		// Update the service to use the new/latest revision of the task
		// definition.
		serviceSublogger.Info("update service to use new task definition")
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
			clusterSublogger.Fatalf("unable to update service to use new task definition: %v", err)
		}

		updatedServiceNames = append(updatedServiceNames, service.ServiceName)
		serviceSublogger.Info("successfully updated service to use new task definition")
	}

	if len(updatedServiceNames) == 0 {
		clusterSublogger.Info("completed deployment of services")

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
	clusterSublogger.Info("completed deployment of services")
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
