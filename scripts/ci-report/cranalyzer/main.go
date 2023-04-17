package main

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"sort"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	_ "github.com/lib/pq"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/scripts/ci-report/report"
)

//go:embed cranalyzer.sql
var prepareDB string

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	logger := slog.Make(sloghuman.Sink(os.Stdout)).Leveled(slog.LevelDebug)

	statsDir := flag.String("stats-dir", "", "Path to ci-stats directory")
	statsGlob := flag.String("glob", "*-stats.json", "Glob to match stats files")
	onlyMain := flag.Bool("only-main", false, "Only import runs on the main branch")
	serve := flag.Bool("serve", true, "Serve db after import")
	dangerous := flag.Bool("dangerous", false, "Do not use transaction and do some imports in parallel")
	flag.Parse()

	if err := run(ctx, logger, *statsDir, *statsGlob, *dangerous, *onlyMain, *serve); err != nil {
		if ctx.Err() == nil {
			logger.Error(ctx, "failed to run cranalyzer", slog.Error(err))
		}
		os.Exit(1)
	}
}

//nolint:revive // We don't care about control flags.
func run(ctx context.Context, logger slog.Logger, statsDir, statsGlob string, dangerous, onlyMain, serve bool) (err error) {
	_, err = os.Stat(statsDir)
	if err != nil && os.IsNotExist(err) {
		return xerrors.Errorf("stats directory does not exist: %w", err)
	}

	u, err := user.Current()
	if err != nil {
		return xerrors.Errorf("failed to get current user: %w", err)
	}
	stdlibLogger := slog.Stdlib(ctx, logger.Named("postgres"), slog.LevelDebug)
	pgPort := 5442
	pgUser := "cranalyzer"
	pgPassword := "cranalyzer"
	pgPath := filepath.Join(u.HomeDir, ".cache", "cranalyzer")
	pgConnectionURL := fmt.Sprintf("postgres://%s@localhost:%d/%s?sslmode=disable&password=%s", pgUser, pgPort, pgUser, pgPassword)
	ep := embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().
			Version(embeddedpostgres.V13).
			BinariesPath(filepath.Join(pgPath, "bin")).
			DataPath(filepath.Join(pgPath, "data")).
			RuntimePath(filepath.Join(pgPath, "runtime")).
			CachePath(filepath.Join(pgPath, "cache")).
			Database(pgUser).
			Username(pgUser).
			Password(pgPassword).
			Port(uint32(pgPort)).
			Logger(stdlibLogger.Writer()),
	)
	err = ep.Start()
	if err != nil {
		return xerrors.Errorf("failed to start embedded postgres: %w", err)
	}
	defer func() {
		if serve && err == nil {
			<-ctx.Done()
		}
		err2 := ep.Stop()
		if err2 != nil && err == nil {
			err = xerrors.Errorf("failed to stop embedded postgres: %w", err2)
		}
	}()

	db, err := sql.Open("postgres", pgConnectionURL)
	if err != nil {
		return xerrors.Errorf("failed to open database connection: %w", err)
	}

	defer db.Close()

	for {
		if err = db.PingContext(ctx); err != nil {
			if ctx.Err() != nil {
				break
			}
			logger.Error(ctx, "failed to ping database", slog.Error(err))
			time.Sleep(100 * time.Millisecond)
			continue
		}
		break
	}

	_, err = db.ExecContext(ctx, prepareDB)
	if err != nil {
		return xerrors.Errorf("failed to prepare database: %w", err)
	}

	_, _ = fmt.Println(pgConnectionURL)

	files, err := filepath.Glob(filepath.Join(statsDir, statsGlob))
	if err != nil {
		return xerrors.Errorf("failed to glob stats files: %w", err)
	}
	// Sort files by name to ensure they're inserted in order of oldest first.
	sort.Strings(files)

	// Try to speed up import via unlogged tables.
	for _, table := range []string{"job_results", "jobs", "runs", "tests"} {
		table := table
		_, err = db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s SET UNLOGGED", table))
		if err != nil {
			return err
		}
		//nolint:revive // This defer is intentional.
		defer func() {
			_, err2 := db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s SET LOGGED", table))
			if err2 != nil && err == nil {
				err = err2
			}
		}()
	}

	db.SetMaxOpenConns(50)

	var tx interface {
		ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
		QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
	}
	if !dangerous {
		tx, err = db.BeginTx(ctx, nil)
		if err != nil {
			return xerrors.Errorf("failed to begin transaction: %w", err)
		}
		defer func() {
			tx, _ := tx.(*sql.Tx)
			if err != nil {
				_ = tx.Rollback()
				return
			}
			err = tx.Commit()
		}()
	} else {
		tx = db
	}

	for _, name := range files {
		name := name
		var s statsJSON
		err = parseJSONFile(name, &s)
		if err != nil {
			return xerrors.Errorf("failed to parse stats file: %q: %w", name, err)
		}

		if onlyMain && s.Branch != "main" {
			logger.Info(ctx, "not main, skip job", slog.F("run_id", s.RunID), slog.F("job_id", s.JobID), slog.F("job", s.Job), slog.F("branch", s.Branch), slog.F("title", s.DisplayTitle))
			continue
		}

		logger.Info(ctx, "processing job", slog.F("run_id", s.RunID), slog.F("job_id", s.JobID), slog.F("job", s.Job), slog.F("branch", s.Branch), slog.F("title", s.DisplayTitle))

		var runID int64
		err = tx.QueryRowContext(ctx, `
			INSERT INTO runs (run_id, author_id, author_login, event, branch, commit, commit_message, ts)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (run_id) DO UPDATE SET run_id = $1
			RETURNING id
			`,
			s.RunID, s.AuthorID, s.AuthorLogin, s.Event, s.Branch, s.SHA, s.DisplayTitle, s.StartedAt,
		).Scan(&runID)
		if err != nil {
			return xerrors.Errorf("failed to insert run: %w", err)
		}

		var jobID int64
		err = tx.QueryRowContext(ctx, `
			INSERT INTO jobs (run_id, job_id, name, ts)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (run_id, job_id) DO NOTHING
			RETURNING id
			`,
			runID, s.JobID, s.Job, s.StartedAt,
		).Scan(&jobID)
		if err != nil {
			// Fast-path, if the job already exists, skip it. Safe because we
			// insert inside a transaction.
			if xerrors.Is(err, sql.ErrNoRows) {
				logger.Info(ctx, "job already exists, skipping", slog.F("run_id", s.RunID))
				continue
			}
			return xerrors.Errorf("failed to insert job: %w", err)
		}

		var eg errgroup.Group
		parallel := 1
		if dangerous {
			parallel = 50
		}
		eg.SetLimit(parallel)

		for _, pkg := range s.Stats.Packages {
			pkg := pkg
			eg.Go(func() error {
				var testID int64
				err = tx.QueryRowContext(ctx, `
				INSERT INTO tests (package, added, last_seen)
				VALUES ($1, $2, $3)
				ON CONFLICT (package) WHERE name IS NULL DO UPDATE SET last_seen = $3
				RETURNING id
				`,
					pkg.Name, s.StartedAt, s.StartedAt,
				).Scan(&testID)
				if err != nil {
					return xerrors.Errorf("failed to insert package: %w", err)
				}

				if pkg.Skip {
					return nil
				}
				status := "fail"
				var duration *float64
				output := &pkg.Output
				if !pkg.Fail {
					status = "pass"
					duration = &pkg.Time
					output = nil
				}
				if pkg.Timeout {
					duration = nil
				}
				_, err = tx.ExecContext(ctx, `
				INSERT INTO job_results (job_id, test_id, status, timeout, execution_time, output)
				VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT DO NOTHING
				`,
					jobID, testID, status, pkg.Timeout, duration, output,
				)
				if err != nil {
					return xerrors.Errorf("failed to insert job result: %w", err)
				}

				return nil
			})
		}
		for _, test := range s.Stats.Tests {
			test := test
			eg.Go(func() error {
				var testID int64
				err = tx.QueryRowContext(ctx, `
				INSERT INTO tests (package, name, added, last_seen)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (package, name) WHERE name IS NOT NULL DO UPDATE SET last_seen = $4
				RETURNING id
				`,
					test.Package, test.Name, s.StartedAt, s.StartedAt,
				).Scan(&testID)
				if err != nil {
					return xerrors.Errorf("failed to insert package: %w", err)
				}

				if test.Skip {
					return nil
				}
				status := "fail"
				var duration *float64
				output := &test.Output
				if !test.Fail {
					status = "pass"
					duration = &test.Time
					output = nil
				}
				if test.Timeout {
					duration = nil
				}
				_, err = tx.ExecContext(ctx, `
				INSERT INTO job_results (job_id, test_id, status, timeout, execution_time, output)
				VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT DO NOTHING
				`,
					jobID, testID, status, test.Timeout, duration, output,
				)
				if err != nil {
					return xerrors.Errorf("failed to insert job result: %w", err)
				}

				return nil
			})
		}
		err = eg.Wait()
		if err != nil {
			return err
		}
	}

	return nil
}

// Output produced by `fetch_stats_from_ci.sh`.
type statsJSON struct {
	AuthorID     int64     `json:"author_id"`
	AuthorLogin  string    `json:"author_login"`
	AuthorEmail  string    `json:"author_email"`
	RunID        int64     `json:"run_id"`
	RunURL       string    `json:"run_url"`
	Event        string    `json:"event"`
	Branch       string    `json:"branch"`
	SHA          string    `json:"sha"`
	StartedAt    string    `json:"started_at"`
	CompletedAt  string    `json:"completed_at"`
	DisplayTitle string    `json:"display_title"`
	JobID        int64     `json:"job_id"`
	Job          string    `json:"job"`
	JobURL       string    `json:"job_url"`
	Stats        report.CI `json:"stats"`
}

func parseJSONFile(path string, v interface{}) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewDecoder(f).Decode(v)
}
