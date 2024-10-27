package dbtestutil

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
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
	var (
		username = "postgres"
		password = "postgres"
		host     = "127.0.0.1"
		port     = "5432"
	)

	dbName, err := cryptorand.StringCharset(cryptorand.Lower, 10)
	if err != nil {
		return "", nil, xerrors.Errorf("generate db name: %w", err)
	}
	dbName = "test_" + dbName

	if err := createDatabaseFromTemplate(CreateDatabaseArgs{
		Username: username,
		Password: password,
		Host:     host,
		Port:     port,
		DBName:   dbName,
	}); err != nil {
		return "", nil, xerrors.Errorf("create database: %w", err)
	}

	cleanup := func() {
		cleanupDbURL := fmt.Sprintf("postgres://%s:%s@%s:%s?sslmode=disable", username, password, host, port)
		cleanupConn, err := sql.Open("postgres", cleanupDbURL)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "cleanup database %q: failed to connect to postgres: %s\n", dbName, err.Error())
		}
		defer cleanupConn.Close()
		_, err = cleanupConn.Exec("DROP DATABASE " + dbName + ";")
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to clean up database %q: %s\n", dbName, err.Error())
		}
	}

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", username, password, host, port, dbName)
	return dsn, cleanup, nil
}

var templateInitLock = sync.Mutex{}

type CreateDatabaseArgs struct {
	Username string
	Password string
	Host     string
	Port     string
	DBName   string
}

// createDatabaseFromTemplate creates a new database from a template database.
// The template database is created if it doesn't exist.
func createDatabaseFromTemplate(args CreateDatabaseArgs) error {
	dbURL := fmt.Sprintf("postgres://%s:%s@%s:%s?sslmode=disable", args.Username, args.Password, args.Host, args.Port)
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return xerrors.Errorf("connect to postgres: %w", err)
	}
	defer db.Close()

	templateDBName := fmt.Sprintf("tpl_%s", migrations.GetMigrationsHash()[:32])
	_, err = db.Exec("CREATE DATABASE " + args.DBName + " WITH TEMPLATE " + templateDBName)
	if err == nil {
		// Template database already exists and we successfully created the new database.
		return nil
	}
	if !(strings.Contains(err.Error(), "template database") && strings.Contains(err.Error(), "does not exist")) {
		return xerrors.Errorf("create db with template: %w", err)
	}

	// We need to create the template database.
	templateInitLock.Lock()
	defer templateInitLock.Unlock()

	// Someone else might have created the template db while we were waiting.
	tplDbExistsRes, err := db.Query("SELECT 1 FROM pg_database WHERE datname = $1", templateDBName)
	if err != nil {
		return xerrors.Errorf("check if db exists: %w", err)
	}
	tplDbAlreadyExists := tplDbExistsRes.Next()
	if !tplDbAlreadyExists {
		// We will use a temporary template database to avoid race conditions. We will
		// rename it to the real template database name after we're sure it was fully
		// initialized.
		// It's dropped here to ensure that if a previous run of this function failed
		// midway, we don't encounter issues with the temporary database still existing.
		tmpTemplateDBName := "tmp_" + templateDBName
		_, err = db.Exec("DROP DATABASE IF EXISTS " + tmpTemplateDBName)
		if err != nil {
			return xerrors.Errorf("drop tmp template db: %w", err)
		}

		_, err = db.Exec("CREATE DATABASE " + tmpTemplateDBName)
		if err != nil {
			return xerrors.Errorf("create tmp template db: %w", err)
		}
		tplDbURL := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", args.Username, args.Password, args.Host, args.Port, tmpTemplateDBName)
		tplDb, err := sql.Open("postgres", tplDbURL)
		if err != nil {
			return xerrors.Errorf("connect to template db: %w", err)
		}
		defer tplDb.Close()
		if err := migrations.Up(tplDb); err != nil {
			return xerrors.Errorf("migrate template db: %w", err)
		}
		if err := tplDb.Close(); err != nil {
			return xerrors.Errorf("close template db: %w", err)
		}
		_, err = db.Exec("ALTER DATABASE " + tmpTemplateDBName + " RENAME TO " + templateDBName)
		if err != nil {
			return xerrors.Errorf("rename tmp template db: %w", err)
		}
	}

	_, err = db.Exec("CREATE DATABASE " + args.DBName + " WITH TEMPLATE " + templateDBName)
	if err != nil {
		return xerrors.Errorf("create db with template after migrations: %w", err)
	}

	return nil
}

func CreateDatabaseTest() error {
	return createDatabaseFromTemplate(CreateDatabaseArgs{
		Username: "postgres",
		Password: "postgres",
		Host:     "127.0.0.1",
		Port:     "5432",
		DBName:   "my_beautiful_db",
	})
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
		Repository: "postgres",
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
