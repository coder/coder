package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"time"

	"golang.org/x/xerrors"

	_ "github.com/lib/pq" // Imported for postgres driver side effects.

	"cdr.dev/slog/v3"
)

const (
	postgresImage    = "postgres"
	postgresTag      = "17"
	postgresUser     = "coder"
	postgresPassword = "coder"
	postgresDB       = "coder"
	postgresPort     = "5432/tcp"
)

// PostgresResult contains the connection info for the running Postgres instance.
type PostgresResult struct {
	// URL is the connection string for the database.
	URL string
	// Port is the host port mapped to the container's 5432.
	Port string
}

var _ Service[PostgresResult] = (*Postgres)(nil)

func OnPostgres() ServiceName {
	return (&Postgres{}).Name()
}

// Postgres runs a PostgreSQL database via docker compose.
type Postgres struct {
	currentStep atomic.Pointer[string]
	result      PostgresResult
}

func (p *Postgres) CurrentStep() string {
	if s := p.currentStep.Load(); s != nil {
		return *s
	}
	return ""
}

func (p *Postgres) setStep(step string) {
	p.currentStep.Store(&step)
}

func NewPostgres() *Postgres {
	return &Postgres{}
}

func (*Postgres) Name() ServiceName {
	return CDevPostgres
}
func (*Postgres) Emoji() string {
	return "🐘"
}

func (*Postgres) DependsOn() []ServiceName {
	return []ServiceName{
		OnDocker(),
	}
}

func (p *Postgres) Start(ctx context.Context, logger slog.Logger, c *Catalog) error {
	defer p.setStep("")

	d, ok := c.MustGet(OnDocker()).(*Docker)
	if !ok {
		return xerrors.New("unexpected type for Docker service")
	}

	p.setStep("Registering database compose service")
	logger.Info(ctx, "registering postgres compose service")

	d.SetComposeVolume("coder_dev_data", ComposeVolume{})
	d.SetCompose("database", ComposeService{
		Image: postgresImage + ":" + postgresTag,
		Environment: map[string]string{
			"POSTGRES_USER":     postgresUser,
			"POSTGRES_PASSWORD": postgresPassword,
			"POSTGRES_DB":       postgresDB,
		},
		Volumes:  []string{"coder_dev_data:/var/lib/postgresql/data"},
		Ports:    []string{"5432:5432"},
		Networks: []string{composeNetworkName},
		Labels:   composeServiceLabels("database"),
		Healthcheck: &ComposeHealthcheck{
			Test:     []string{"CMD-SHELL", "pg_isready -U coder"},
			Interval: "2s",
			Timeout:  "5s",
			Retries:  10,
		},
	})

	p.setStep("Starting PostgreSQL via compose")
	if err := d.DockerComposeUp(ctx, "database"); err != nil {
		return xerrors.Errorf("docker compose up database: %w", err)
	}

	// Fixed port mapping via compose.
	p.result = PostgresResult{
		URL:  fmt.Sprintf("postgres://%s:%s@localhost:5432/%s?sslmode=disable", postgresUser, postgresPassword, postgresDB),
		Port: "5432",
	}

	p.setStep("Waiting for PostgreSQL to be ready")
	return p.waitForReady(ctx, logger)
}

func (p *Postgres) sqlDB() (*sql.DB, error) {
	db, err := sql.Open("postgres", p.result.URL)
	if err != nil {
		return nil, xerrors.Errorf("open database: %w", err)
	}
	return db, nil
}

// waitForMigrations polls the schema_migrations table until
// migrations are complete (version != 0 and dirty = false).
// This is necessary because EnsureLicense may run concurrently
// with coderd's startup, which performs migrations.
func (p *Postgres) waitForMigrations(ctx context.Context, logger slog.Logger) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	timeout := time.After(5 * time.Minute)

	db, err := sql.Open("postgres", p.result.URL)
	if err != nil {
		return xerrors.Errorf("open database: %w", err)
	}
	defer db.Close()

	for {
		var version int64
		var dirty bool
		err := db.QueryRowContext(ctx,
			"SELECT version, dirty FROM schema_migrations LIMIT 1",
		).Scan(&version, &dirty)
		if err == nil && version != 0 && !dirty {
			logger.Info(ctx, "migrations complete",
				slog.F("version", version),
			)
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return xerrors.New("timed out waiting for migrations")
		case <-ticker.C:
		}
	}
}

func (p *Postgres) waitForReady(ctx context.Context, logger slog.Logger) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(60 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return xerrors.New("timeout waiting for postgres to be ready")
		case <-ticker.C:
			db, err := sql.Open("postgres", p.result.URL)
			if err != nil {
				continue
			}
			err = db.PingContext(ctx)
			_ = db.Close()
			if err == nil {
				logger.Info(ctx, "postgres is ready", slog.F("url", p.result.URL))
				return nil
			}
		}
	}
}

func (*Postgres) Stop(_ context.Context) error {
	// Don't stop the container - it persists across runs.
	// Use "cdev down" to fully clean up.
	return nil
}

func (p *Postgres) Result() PostgresResult {
	return p.result
}
