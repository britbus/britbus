package main

import (
	"github.com/travigo/travigo/pkg/api"
	"github.com/travigo/travigo/pkg/dataimporter"
	"github.com/travigo/travigo/pkg/realtime"
	stats "github.com/travigo/travigo/pkg/stats/cli"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/travigo/travigo/pkg/transforms"
	"github.com/urfave/cli/v2"

	_ "time/tzdata"
)

func main() {
	if os.Getenv("TRAVIGO_LOG_FORMAT") != "JSON" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
	}

	if os.Getenv("TRAVIGO_DEBUG") == "YES" {
		log.Logger = log.Logger.Level(zerolog.DebugLevel)
	} else {
		log.Logger = log.Logger.Level(zerolog.InfoLevel)
	}

	transforms.SetupClient()

	app := &cli.App{
		Name:        "travigo",
		Description: "Single binary of truth for Travigo - runs all the services",

		Commands: []*cli.Command{
			dataimporter.RegisterCLI(),
			api.RegisterCLI(),
			realtime.RegisterCLI(),
			stats.RegisterCLI(),
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal().Err(err).Send()
	}
}