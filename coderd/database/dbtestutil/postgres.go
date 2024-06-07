package dbtestutil

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/cryptorand"
)

// Open creates a new PostgreSQL database instance.  With DB_FROM environment variable set, it clones a database
// from the provided template.  With the environment variable unset, it creates a new Docker container running postgres.
func Open() (string, func(), error) {
	if os.Getenv("DB_FROM") != "" {
		// In CI, creating a Docker container for each test is slow.
		// This expects a PostgreSQL instance with the hardcoded credentials
		// available.
		dbURL := "postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable"
		db, err := sql.Open("postgres", dbURL)
		if err != nil {
			return "", nil, xerrors.Errorf("connect to ci postgres: %w", err)
		}

		defer db.Close()

		dbName, err := cryptorand.StringCharset(cryptorand.Lower, 10)
		if err != nil {
			return "", nil, xerrors.Errorf("generate db name: %w", err)
		}

		dbName = "ci" + dbName
		_, err = db.Exec("CREATE DATABASE " + dbName + " WITH TEMPLATE " + os.Getenv("DB_FROM"))
		if err != nil {
			return "", nil, xerrors.Errorf("create db with template: %w", err)
		}

		dsn := "postgres://postgres:postgres@127.0.0.1:5432/" + dbName + "?sslmode=disable"
		// Normally this would get cleaned up by removing the container but if we
		// reuse the same container for multiple tests we run the risk of filling
		// up our disk. Avoid this!
		cleanup := func() {
			cleanupConn, err := sql.Open("postgres", dbURL)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "cleanup database %q: failed to connect to postgres: %s\n", dbName, err.Error())
			}
			defer cleanupConn.Close()
			_, err = cleanupConn.Exec("DROP DATABASE " + dbName + ";")
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "failed to clean up database %q: %s\n", dbName, err.Error())
			}
		}
		return dsn, cleanup, nil
	}
	return OpenContainerized(0)
}

// OpenContainerized creates a new PostgreSQL server using a Docker container.  If port is nonzero, forward host traffic
// to that port to the database.  If port is zero, allocate a free port from the OS.
func OpenContainerized(port int) (string, func(), error) {
	pool, err := dockertest.NewPool("")
	if err != nil {
		return "", nil, xerrors.Errorf("create pool: %w", err)
	}

	tempDir, err := os.MkdirTemp(os.TempDir(), "postgres")
	if err != nil {
		return "", nil, xerrors.Errorf("create tempdir: %w", err)
	}

	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "gcr.io/coder-dev-1/postgres",
		Tag:        "13",
		Env: []string{
			"POSTGRES_PASSWORD=postgres",
			"POSTGRES_USER=postgres",
			"POSTGRES_DB=postgres",
			// The location for temporary database files!
			"PGDATA=/tmp",
			"listen_addresses = '*'",
		},
		PortBindings: map[docker.Port][]docker.PortBinding{
			"5432/tcp": {{
				// Manually specifying a host IP tells Docker just to use an IPV4 address.
				// If we don't do this, we hit a fun bug:
				// https://github.com/moby/moby/issues/42442
				// where the ipv4 and ipv6 ports might be _different_ and collide with other running docker containers.
				HostIP:   "0.0.0.0",
				HostPort: strconv.FormatInt(int64(port), 10),
			}},
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
		return "", nil, xerrors.Errorf("expire resource: %w", err)
	}

	pool.MaxWait = 120 * time.Second

	// Record the error that occurs during the retry.
	// The 'pool' pkg hardcodes a deadline error devoid
	// of any useful context.
	var retryErr error
	err = pool.Retry(func() error {
		db, err := sql.Open("postgres", dbURL)
		if err != nil {
			retryErr = xerrors.Errorf("open postgres: %w", err)
			return retryErr
		}
		defer db.Close()

		err = db.Ping()
		if err != nil {
			retryErr = xerrors.Errorf("ping postgres: %w", err)
			return retryErr
		}

		err = migrations.Up(db)
		if err != nil {
			retryErr = xerrors.Errorf("migrate db: %w", err)
			// Only try to migrate once.
			return backoff.Permanent(retryErr)
		}

		return nil
	})
	if err != nil {
		return "", nil, retryErr
	}

	return dbURL, func() {
		_ = pool.Purge(resource)
		_ = os.RemoveAll(tempDir)
	}, nil
}
