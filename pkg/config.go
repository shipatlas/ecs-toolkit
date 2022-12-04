package pkg

type Config struct {
	Version  *string    `mapstructure:"version" validate:"required"`
	Cluster  *string    `mapstructure:"cluster" validate:"required"`
	Services []*Service `mapstructure:"services" validate:"dive"`
	Tasks    []*Task    `mapstructure:"tasks" validate:"dive"`
}

type Service struct {
	Name       *string   `mapstructure:"name" validate:"required"`
	Containers []*string `mapstructure:"containers" validate:"required,min=1,dive"`
	Force      *bool     `mapstructure:"force"`
}

type Task struct {
	Family     *string   `mapstructure:"family" validate:"required"`
	Containers []*string `mapstructure:"containers" validate:"required,min=1,dive"`
	Force      *bool     `mapstructure:"force"`

	CapacityProviderStrategies []*CapacityProviderStrategy `mapstructure:"capacity_provider_strategies"`
	Count                      *int32                      `mapstructure:"count" validate:"required"`
	LaunchType                 *string                     `mapstructure:"launch_type" validate:"omitempty,oneof=ec2 fargate external"`
	NetworkConfiguration       *NetworkConfiguration       `mapstructure:"network_configuration"`
}

type CapacityProviderStrategy struct {
	CapacityProvider *string `mapstructure:"capacity_provider" validate:"required"`
	Base             *int32  `mapstructure:"base" validate:"required"`
	Weight           *int32  `mapstructure:"weight" validate:"required"`
}

type NetworkConfiguration struct {
	VpcConfiguration *VpcConfiguration `mapstructure:"vpc_configuration" validate:"required,dive"`
}

type VpcConfiguration struct {
	AssignPublicIp *bool     `mapstructure:"assign_public_ip" validate:"required"`
	SecurityGroups []*string `mapstructure:"security_groups" validate:"required,min=1"`
	Subnets        []*string `mapstructure:"subnets" validate:"required,min=1"`
}

func (config *Config) ServiceNames() []string {
	serviceNames := []string{}
	for _, service := range config.Services {
		serviceNames = append(serviceNames, *service.Name)
	}

	return serviceNames
}
