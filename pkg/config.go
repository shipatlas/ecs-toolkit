package pkg

type Config struct {
	Version     *string      `mapstructure:"version" validate:"required"`
	Application *Application `mapstructure:"application" validate:"required,dive"`
}

type Application struct {
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
	Name       *string   `mapstructure:"name" validate:"required"`
	Containers []*string `mapstructure:"containers" validate:"required,min=1,dive"`
	Force      *bool     `mapstructure:"force"`
}
