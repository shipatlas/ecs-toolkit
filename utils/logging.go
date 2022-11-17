package utils

import (
	log "github.com/sirupsen/logrus"
)

var (
	// LogLevels is a list of valid log levels.
	LogLevels = []string{"trace", "debug", "info", "warn", "error", "fatal", "panic"}
)

// SetLogLevel sets the level at which to log messages.
func SetLogLevel(level string) {
	switch level {
	case "trace":
		log.SetLevel(log.TraceLevel)
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	case "fatal":
		log.SetLevel(log.FatalLevel)
	case "panic":
		log.SetLevel(log.PanicLevel)
	default:
		log.Fatal("invalid logging level")
	}

	log.Debugf("log level set to %s", level)
}
