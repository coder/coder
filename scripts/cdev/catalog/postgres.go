package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ory/dockertest/v3/docker"

	_ "github.com/lib/pq"

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

func OnPostgres() string {
	return (&Postgres{}).Name()
}

// Postgres runs a PostgreSQL database in a Docker container.
type Postgres struct {
	containerID string
	result      PostgresResult
}

func NewPostgres() *Postgres {
	return &Postgres{}
}

func (p *Postgres) Name() string {
	return "postgres"
}
func (p *Postgres) Emoji() string {
	return "🐘"
}

func (p *Postgres) DependsOn() []string {
	return []string{
		OnDocker(),
	}
}

func (p *Postgres) Start(ctx context.Context, c *Catalog) error {
	logger := c.ServiceLogger(p.Name())
	d := c.MustGet(OnDocker()).(*Docker)
	pool := d.Result()

	name := "cdev_postgres"
	labels := NewServiceLabels(CDevPostgres)
	filter := labels.Filter()
	filter["name"] = []string{name}

	// Check if container already exists and is running.
	containers, err := pool.Client.ListContainers(docker.ListContainersOptions{
		Filters: filter,
	})
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}

	if len(containers) > 0 {
		// Reuse existing container.
		container := containers[0]
		p.containerID = container.ID
		for _, port := range container.Ports {
			if port.PrivatePort == 5432 {
				p.result = PostgresResult{
					URL:  fmt.Sprintf("postgres://%s:%s@localhost:%d/%s?sslmode=disable", postgresUser, postgresPassword, port.PublicPort, postgresDB),
					Port: fmt.Sprintf("%d", port.PublicPort),
				}
				break
			}
		}
		logger.Info(ctx, "reusing existing postgres container", slog.F("container_id", p.containerID[:12]))
		return p.waitForReady(ctx, logger)
	}

	// Ensure data volume exists.
	vol, err := d.EnsureVolume(ctx, VolumeOptions{
		Name:   "cdev_postgres_data",
		Labels: labels.With(CDevLabelCache, "true"),
	})
	if err != nil {
		return fmt.Errorf("ensure postgres volume: %w", err)
	}

	logger.Info(ctx, "starting postgres container")

	cntSink := NewLoggerSink(c.w, p)
	cntLogger := slog.Make(cntSink)
	// This stops all postgres logs from dumping to the console. We only want the
	// logs until the database is ready, after that we can let it log as normal since
	// it's running in detached mode.
	defer cntSink.Close()

	// Start new container.
	result, err := RunContainer(ctx, pool, CDevPostgres, ContainerRunOptions{
		CreateOpts: docker.CreateContainerOptions{
			Name: name,
			Config: &docker.Config{
				Image: postgresImage + ":" + postgresTag,
				Env: []string{
					"POSTGRES_USER=" + postgresUser,
					"POSTGRES_PASSWORD=" + postgresPassword,
					"POSTGRES_DB=" + postgresDB,
				},
				Labels: labels,
			},
			HostConfig: &docker.HostConfig{
				Binds: []string{
					fmt.Sprintf("%s:/var/lib/postgresql/data", vol.Name),
				},
				RestartPolicy: docker.RestartPolicy{Name: "unless-stopped"},
				PortBindings: map[docker.Port][]docker.PortBinding{
					postgresPort: {{HostIP: "127.0.0.1", HostPort: ""}},
				},
			},
			NetworkingConfig: nil,
			Context:          nil,
		},
		Logger:   cntLogger,
		Detached: true,
		Stdout:   nil,
		Stderr:   nil,
	})
	if err != nil {
		return fmt.Errorf("run container: %w", err)
	}

	// The networking port takes some time to be available.
	// Ideally there is more reusable "Wait" style code.
	timeout, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	for {
		if timeout.Err() != nil {
			return fmt.Errorf("timeout waiting for postgres container to start: %w", timeout.Err())
		}
		if len(result.Container.NetworkSettings.Ports["5432/tcp"]) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	p.containerID = result.Container.ID
	hostPort := result.Container.NetworkSettings.Ports["5432/tcp"][0].HostPort
	p.result = PostgresResult{
		URL:  fmt.Sprintf("postgres://%s:%s@localhost:%s/%s?sslmode=disable", postgresUser, postgresPassword, hostPort, postgresDB),
		Port: hostPort,
	}

	return p.waitForReady(ctx, logger)
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
			return fmt.Errorf("timeout waiting for postgres to be ready")
		case <-ticker.C:
			db, err := sql.Open("postgres", p.result.URL)
			if err != nil {
				continue
			}
			err = db.PingContext(ctx)
			db.Close()
			if err == nil {
				logger.Info(ctx, "postgres is ready", slog.F("url", p.result.URL))
				return nil
			}
		}
	}
}

func (p *Postgres) Stop(_ context.Context) error {
	// Don't stop the container - it persists across runs.
	// Use "cdev down" to fully clean up.
	return nil
}

func (p *Postgres) Result() PostgresResult {
	return p.result
}
