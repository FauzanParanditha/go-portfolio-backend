package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func Init(env string) {
	zerolog.TimeFieldFormat = time.RFC3339

	if env == "development" {
		consoleWriter := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
		log.Logger = zerolog.New(consoleWriter).With().Timestamp().Logger()
	} else {
		log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}
}
