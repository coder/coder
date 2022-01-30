package postgres

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"os"
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
	tempDir, err := ioutil.TempDir(os.TempDir(), "postgres")
	if err != nil {
		return "", nil, xerrors.Errorf("create tempdir: %w", err)
	}
	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "11",
		Env: []string{
			"POSTGRES_PASSWORD=postgres",
			"POSTGRES_USER=postgres",
			"POSTGRES_DB=postgres",
			// The location for temporary database files!
			"PGDATA=/tmp",
			"listen_addresses = '*'",
		},
		Mounts: []string{
			// The postgres image has a VOLUME parameter in it's image.
			// If we don't mount at this point, Docker will allocate a
			// volume for this directory.
			//
			// This isn't used anyways, since we override PGDATA.
			fmt.Sprintf("%s:/var/lib/postgresql/data", tempDir),
		},
	}, func(config *docker.HostConfig) {
		// set AutoRemove to true so that stopped container goes away by itself
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		return "", nil, xerrors.Errorf("could not start resource: %w", err)
	}
	hostAndPort := resource.GetHostPort("5432/tcp")
	dbURL := fmt.Sprintf("postgres://postgres:postgres@%s/postgres?sslmode=disable", hostAndPort)

	// Docker should hard-kill the container after 120 seconds.
	err = resource.Expire(120)
	if err != nil {
		return "", nil, xerrors.Errorf("could not expire resource: %w", err)
	}

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
		_ = os.RemoveAll(tempDir)
	}, nil
}
