package catalog

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

const (
	defaultDBPort = "5432"
	defaultDBUser = "coder"
	defaultDBPass = "coder"
	defaultDBName = "coder"
	containerName = "cdev-postgres"
	postgresImage = "postgres:16"
)

// Database runs a PostgreSQL container for local development.
type Database struct {
	Port     string
	User     string
	Password string
	DBName   string

	connURL string
}

func NewDatabase() *Database {
	return &Database{
		Port:     defaultDBPort,
		User:     defaultDBUser,
		Password: defaultDBPass,
		DBName:   defaultDBName,
	}
}

func (d *Database) Name() string {
	return DatabaseName
}

func (d *Database) DependsOn() []string {
	return nil // No dependencies.
}

func (d *Database) EnablementFlag() string {
	return "--database"
}

func (d *Database) ConnectionURL() string {
	return d.connURL
}

func (d *Database) Start(ctx context.Context) error {
	// Check if container already exists and is running.
	checkCmd := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", containerName)
	if out, err := checkCmd.Output(); err == nil && string(out) == "true\n" {
		d.connURL = d.buildConnURL()
		return nil // Already running.
	}

	// Remove any stopped container with the same name.
	_ = exec.CommandContext(ctx, "docker", "rm", "-f", containerName).Run()

	// Start the container.
	//nolint:gosec
	runCmd := exec.CommandContext(ctx, "docker", "run",
		"-d",
		"--name", containerName,
		"--label", "cdev=true",
		"-p", fmt.Sprintf("%s:5432", d.Port),
		"-e", fmt.Sprintf("POSTGRES_USER=%s", d.User),
		"-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", d.Password),
		"-e", fmt.Sprintf("POSTGRES_DB=%s", d.DBName),
		postgresImage,
	)
	if out, err := runCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start postgres: %w: %s", err, out)
	}

	d.connURL = d.buildConnURL()

	// Wait for postgres to be ready.
	return d.waitReady(ctx)
}

func (d *Database) Stop(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "stop", containerName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to stop postgres: %w: %s", err, out)
	}
	return nil
}

func (d *Database) Healthy(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "exec", containerName,
		"pg_isready", "-U", d.User, "-d", d.DBName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("postgres not ready: %w", err)
	}
	return nil
}

func (d *Database) buildConnURL() string {
	return fmt.Sprintf("postgres://%s:%s@localhost:%s/%s?sslmode=disable",
		d.User, d.Password, d.Port, d.DBName)
}

func (d *Database) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if d.Healthy(ctx) == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("postgres failed to become ready within 30s")
}
