package cli

import (
	"database/sql"
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/migrations"
)

func (*RootCmd) appSecurityKey() *clibase.Cmd {
	var postgresURL string

	root := &clibase.Cmd{
		Use:   "app-security-key",
		Short: "Directly connect to the database to print the app security key",
		// We can unhide this if we decide to keep it this way.
		Hidden:     true,
		Middleware: clibase.RequireNArgs(0),
		Handler: func(inv *clibase.Invocation) error {
			sqlDB, err := sql.Open("postgres", postgresURL)
			if err != nil {
				return xerrors.Errorf("dial postgres: %w", err)
			}
			defer sqlDB.Close()
			err = sqlDB.Ping()
			if err != nil {
				return xerrors.Errorf("ping postgres: %w", err)
			}

			err = migrations.EnsureClean(sqlDB)
			if err != nil {
				return xerrors.Errorf("database needs migration: %w", err)
			}
			db := database.New(sqlDB)

			key, err := db.GetAppSecurityKey(inv.Context())
			if err != nil {
				return xerrors.Errorf("retrieving key: %w", err)
			}

			_, _ = fmt.Fprintln(inv.Stdout, key)
			return nil
		},
	}

	root.Options = clibase.OptionSet{
		{
			Flag:        "postgres-url",
			Description: "URL of a PostgreSQL database to connect to.",
			Env:         "CODER_PG_CONNECTION_URL",
			Value:       clibase.StringOf(&postgresURL),
		},
	}

	return root
}
