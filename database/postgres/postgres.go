package postgres

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"golang.org/x/xerrors"
)

// Open creates a new PostgreSQL server using a Docker container.
func Open() (string, func(), error) {
	pool, err := dockertest.NewPool("")
	if err != nil {
		return "", nil, xerrors.Errorf("create pool: %w", err)
	}
	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "11",
		Env: []string{
			"POSTGRES_PASSWORD=postgres",
			"POSTGRES_USER=postgres",
			"POSTGRES_DB=postgres",
			"listen_addresses = '*'",
		},
	}, func(config *docker.HostConfig) {
		// set AutoRemove to true so that stopped container goes away by itself
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		log.Fatalf("Could not start resource: %s", err)
	}
	hostAndPort := resource.GetHostPort("5432/tcp")
	dbURL := fmt.Sprintf("postgres://postgres:postgres@%s/postgres?sslmode=disable", hostAndPort)

	// Docker should hard-kill the container after 120 seconds.
	resource.Expire(120)

	pool.MaxWait = 120 * time.Second
	err = pool.Retry(func() error {
		db, err := sql.Open("postgres", dbURL)
		if err != nil {
			return err
		}
		err = db.Ping()
		_ = db.Close()
		return err
	})
	if err != nil {
		return "", nil, err
	}
	return dbURL, func() {
		_ = pool.Purge(resource)
	}, nil
}
