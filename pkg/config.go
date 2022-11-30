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
}

func (config *Config) ServiceNames() []string {
	serviceNames := []string{}
	for _, service := range config.Services {
		serviceNames = append(serviceNames, *service.Name)
	}

	return serviceNames
}
