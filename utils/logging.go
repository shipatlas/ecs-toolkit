package utils

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	// LogLevels is a list of valid log levels.
	LogLevels = []string{"trace", "debug", "info", "warn", "error", "fatal", "panic"}
)

// SetLogLevel sets the level at which to log messages.
func SetLogLevel(level string) {
	switch level {
	case "trace":
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	case "fatal":
		zerolog.SetGlobalLevel(zerolog.FatalLevel)
	case "panic":
		zerolog.SetGlobalLevel(zerolog.PanicLevel)
	default:
		log.Fatal().Msg("invalid logging level")
	}

	log.Debug().Msgf("log level set to %s", level)
}
