package pkg

type Config struct {
	Version     *string      `mapstructure:"version"`
	Application *Application `mapstructure:"application"`
}

type Application struct {
	Services []*Service `mapstructure:"services"`
	Tasks    []*Task    `mapstructure:"tasks"`
}

type Service struct {
	Name *string `mapstructure:"name"`
}

type Task struct {
	Name *string `mapstructure:"name"`
}
