package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/serpent"
)

func main() {
	var (
		logLevel    string
		postgresURL string
	)
	cmd := serpent.Command{
		Use:   "generatedata",
		Short: "Generate sample data for a Coder installation",
		Options: serpent.OptionSet{
			{
				Name:          "log-level",
				Description:   "What level of logs to output.",
				Required:      false,
				Flag:          "log-level",
				FlagShorthand: "",
				Default:       "info",
				Value:         serpent.StringOf(&logLevel),
			},
			{
				Name:          "postgres-url",
				Description:   "Postgres connection URL.",
				Required:      true,
				Flag:          "postgres-url",
				FlagShorthand: "",
				Value:         serpent.StringOf(&postgresURL),
			},
		},
		Handler: func(i *serpent.Invocation) error {
			ctx := i.Context()

			if postgresURL == "" {
				return xerrors.New("postgres-url is required")
			}

			logger := slog.Make(sloghuman.Sink(os.Stderr))
			switch strings.ToLower(logLevel) {
			case "debug":
				logger = logger.Leveled(slog.LevelDebug)
			case "info":
				logger = logger.Leveled(slog.LevelInfo)
			case "warn":
				logger = logger.Leveled(slog.LevelWarn)
			case "error":
				logger = logger.Leveled(slog.LevelError)
			default:
				return xerrors.New("invalid log level")
			}

			sqlDB, err := cli.ConnectToPostgres(ctx, logger, "postgres", postgresURL, migrations.Up)
			if err != nil {
				return xerrors.Errorf("connect to postgres: %w", err)
			}
			defer sqlDB.Close()

			db := database.New(sqlDB)
			Generate(func(t *testing.T) {
				// Time to generate data!
				for i := 0; i < 10; i++ {
					dbgen.Organization(t, db, database.Organization{})
				}
			})
			var _ = db
			return nil
		},
	}

	if err := cmd.Invoke().WithOS().Run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %+v\n", err)
		os.Exit(1)
	}
}

func Generate(do func(t *testing.T)) {
	//testing.Init()
	//_ = flag.Set("test.timeout", "0")

	// This is just a way to run tests outside go test
	testing.Main(func(_, _ string) (bool, error) {
		return true, nil
	}, []testing.InternalTest{
		{
			Name: "Run Fake IDP",
			F:    do,
		},
	}, nil, nil)
}
