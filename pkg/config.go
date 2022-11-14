package pkg

type Config struct {
	Version     *string      `mapstructure:"version" validate:"required"`
	Application *Application `mapstructure:"application" validate:"required"`
}

type Application struct {
	Services []*Service `mapstructure:"services"`
	Tasks    []*Task    `mapstructure:"tasks"`
}

type Service struct {
	Name *string `mapstructure:"name" validate:"required"`
}

type Task struct {
	Name *string `mapstructure:"name" validate:"required"`
}
