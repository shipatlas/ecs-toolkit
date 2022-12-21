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
	"strings"

	"github.com/go-playground/validator/v10"

	log "github.com/sirupsen/logrus"
)

type Config struct {
	Version string `mapstructure:"version" validate:"required,oneof=v1"`
	Cluster string `mapstructure:"cluster" validate:"required"`

	Services []Service `mapstructure:"services" validate:"omitempty,dive"`
	Tasks    Tasks     `mapstructure:"tasks" validate:"omitempty,dive"`
}

type Service struct {
	Name       string   `mapstructure:"name" validate:"required"`
	Containers []string `mapstructure:"containers" validate:"required,min=1,dive"`

	Force *bool `mapstructure:"force"`
}

type Task struct {
	Family     string   `mapstructure:"family" validate:"required"`
	Containers []string `mapstructure:"containers" validate:"required,min=1,dive"`
	Count      int32    `mapstructure:"count" validate:"required,min=1,max=10"`

	CapacityProviderStrategies []CapacityProviderStrategy `mapstructure:"capacity_provider_strategies" validate:"omitempty,max=6,dive"`
	LaunchType                 *string                    `mapstructure:"launch_type" validate:"omitempty,oneof=ec2 fargate external"`
	NetworkConfiguration       *NetworkConfiguration      `mapstructure:"network_configuration" validate:"omitempty,dive"`
}

type Tasks struct {
	Pre  []Task `mapstructure:"pre" validate:"omitempty,dive"`
	Post []Task `mapstructure:"post" validate:"omitempty,dive"`
}

type TaskStage string

type CapacityProviderStrategy struct {
	CapacityProvider string `mapstructure:"capacity_provider" validate:"required"`
	Base             int32  `mapstructure:"base"`
	Weight           int32  `mapstructure:"weight"`
}

type NetworkConfiguration struct {
	VpcConfiguration VpcConfiguration `mapstructure:"vpc_configuration" validate:"required,dive"`
}

type VpcConfiguration struct {
	AssignPublicIP bool     `mapstructure:"assign_public_ip" validate:"required"`
	SecurityGroups []string `mapstructure:"security_groups" validate:"required,min=1,max=5,dive"`
	Subnets        []string `mapstructure:"subnets" validate:"required,min=1,max=16,dive"`
}

const (
	TaskStagePost TaskStage = "post"
	TaskStagePre  TaskStage = "pre"
)

func (config *Config) Validate() error {
	validate := validator.New()
	err := validate.Struct(config)
	if err != nil {
		if _, ok := err.(*validator.InvalidValidationError); ok {
			log.Error(strings.ToLower(err.Error()))
		}

		for _, err := range err.(validator.ValidationErrors) {
			log.Error(strings.ToLower(err.Error()))
		}

		return err
	}

	return nil
}
