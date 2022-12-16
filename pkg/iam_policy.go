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
	"encoding/json"
	"fmt"

	log "github.com/sirupsen/logrus"
)

type PolicyDocument struct {
	Version   string           `json:"Version"`
	Statement []StatementEntry `json:"Statement"`
}

type StatementEntry struct {
	Sid      string      `json:"Sid"`
	Effect   string      `json:"Effect"`
	Action   interface{} `json:"Action"`
	Resource interface{} `json:"Resource"`
}

func (config *Config) GenerateIAMPolicy(account, region string) (string, error) {
	var (
		serviceArns               []string
		serviceNames              []string
		serviceTaskDefinitionArns []string
		taskFamilies              []string
		taskTaskDefinitionArns    []string
	)

	serviceNames = config.ServiceNames()
	taskFamilies = config.TaskFamilies()

	for _, serviceName := range serviceNames {
		serviceArn := fmt.Sprintf("arn:aws:ecs:%s:%s:service/%s/%s", region, account, *config.Cluster, serviceName)
		serviceArns = append(serviceArns, serviceArn)
	}

	for _, serviceName := range serviceNames {
		taskDefinitionFamilyArn := fmt.Sprintf("arn:aws:ecs:%s:%s:task-definition/%s:*", region, account, serviceName)
		serviceTaskDefinitionArns = append(serviceTaskDefinitionArns, taskDefinitionFamilyArn)
	}

	for _, taskFamily := range taskFamilies {
		taskDefinitionFamilyArn := fmt.Sprintf("arn:aws:ecs:%s:%s:task-definition/%s:*", region, account, taskFamily)
		taskTaskDefinitionArns = append(taskTaskDefinitionArns, taskDefinitionFamilyArn)
	}

	taskDefinitionFamilyArns := append(serviceTaskDefinitionArns, taskTaskDefinitionArns...)

	policy := PolicyDocument{
		Version: "2012-10-17",
		Statement: []StatementEntry{
			StatementEntry{
				Sid:    "AccessServices",
				Effect: "Allow",
				Action: []string{
					"ecs:DescribeServices",
					"ecs:UpdateService",
				},
				Resource: serviceArns,
			},
			StatementEntry{
				Sid:    "AccessTaskDefinitions",
				Effect: "Allow",
				Action: []string{
					"ecs:DescribeTaskDefinition",
					"ecs:RegisterTaskDefinition",
				},
				Resource: taskDefinitionFamilyArns,
			},
			StatementEntry{
				Sid:      "AccessTasks",
				Effect:   "Allow",
				Action:   "ecs:DescribeTasks",
				Resource: fmt.Sprintf("arn:aws:ecs:%s:%s:task/%s/*", region, account, *config.Cluster),
			},
			StatementEntry{
				Sid:      "RunTasks",
				Effect:   "Allow",
				Action:   "ecs:RunTask",
				Resource: taskTaskDefinitionArns,
			},
		},
	}

	policyBytes, err := json.MarshalIndent(&policy, "", "  ")
	if err != nil {
		log.Errorf("error marshaling policy: %v", err)

		return "", err
	}

	return string(policyBytes), nil
}
