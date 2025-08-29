package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/migrations"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %+v\n", err)
		os.Exit(1)
	}
}

func run() error {
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	postgresURL := flag.String("postgres-url", "", "Postgres connection URL")

	flag.Parse()
	ctx := context.Background()

	if *postgresURL == "" {
		return xerrors.New("postgres-url is required")
	}

	logger := slog.Make(sloghuman.Sink(os.Stderr))
	switch strings.ToLower(*logLevel) {
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

	sqlDB, err := cli.ConnectToPostgres(ctx, logger, "postgres", *postgresURL, migrations.Up)
	if err != nil {
		return xerrors.Errorf("connect to postgres: %w", err)
	}
	defer sqlDB.Close()

	db := database.New(sqlDB)
	Generate(db, GenerateWorkspaceBuilds)
	return nil
}

// GenerateWorkspaceBuilds generates many workspace builds for many workspaces.
func GenerateWorkspaceBuilds(t *testing.T, db database.Store) {
	err := db.InTx(func(db database.Store) error {
		ctx := context.Background()
		o := pe(db.GetDefaultOrganization(ctx))
		admin := dbgen.User(t, db, database.User{})

		tv := dbfake.TemplateVersion(t, db).
			Seed(database.TemplateVersion{
				OrganizationID: o.ID,
				CreatedBy:      admin.ID,
			}).
			Do()

		users := make([]database.User, 10)
		for i := range users {
			users[i] = dbgen.User(t, db, database.User{})
			dbgen.OrganizationMember(t, db, database.OrganizationMember{
				UserID:         users[i].ID,
				OrganizationID: o.ID,
			})

			ws := dbgen.Workspace(t, db, database.WorkspaceTable{
				TemplateID: tv.Template.ID,
				OwnerID:    users[i].ID,
			})
			for j := 0; j < 100; j++ {
				dbfake.WorkspaceBuild(t, db, ws).
					Seed(database.WorkspaceBuild{
						WorkspaceID: ws.ID,
						BuildNumber: int32(j) + 1,
					}).
					Do()
			}
		}
		return nil
	}, nil)
	if err != nil {
		t.Fatalf("failed to run in transaction: %+v", err)
	}
}

func pe[T any](thing T, err error) T {
	if err != nil {
		panic(err)
	}
	return thing
}

func Generate(db database.Store, do func(t *testing.T, db database.Store)) {
	//testing.Init()
	//_ = flag.Set("test.timeout", "0")

	// This is just a way to run tests outside go test
	testing.Main(func(_, _ string) (bool, error) {
		return true, nil
	}, []testing.InternalTest{
		{
			Name: "Run data generation",
			F: func(t *testing.T) {
				do(t, db)
			},
		},
	}, nil, nil)
}
