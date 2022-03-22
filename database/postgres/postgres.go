package postgres

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"sync"
	"time"

	"github.com/coder/coder/cryptorand"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"golang.org/x/xerrors"
)

// Required to prevent port collision during container creation.
// Super unlikely, but it happened. See: https://github.com/coder/coder/runs/5375197003
var openPortMutex sync.Mutex

// Open creates a new PostgreSQL server using a Docker container.
func Open() (string, func(), error) {
	if os.Getenv("DB") == "ci" {
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
		_, err = db.Exec("CREATE DATABASE " + dbName)
		if err != nil {
			return "", nil, xerrors.Errorf("create db: %w", err)
		}
		return "postgres://postgres:postgres@127.0.0.1:5432/" + dbName + "?sslmode=disable", func() {}, nil
	}

	pool, err := dockertest.NewPool("")
	if err != nil {
		return "", nil, xerrors.Errorf("create pool: %w", err)
	}
	tempDir, err := ioutil.TempDir(os.TempDir(), "postgres")
	if err != nil {
		return "", nil, xerrors.Errorf("create tempdir: %w", err)
	}
	openPortMutex.Lock()
	// Pick an explicit port on the host to connect to 5432.
	// This is necessary so we can configure the port to only use ipv4.
	port, err := getFreePort()
	if err != nil {
		openPortMutex.Unlock()
		return "", nil, xerrors.Errorf("Unable to get free port: %w", err)
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
		PortBindings: map[docker.Port][]docker.PortBinding{
			"5432/tcp": {{
				// Manually specifying a host IP tells Docker just to use an IPV4 address.
				// If we don't do this, we hit a fun bug:
				// https://github.com/moby/moby/issues/42442
				// where the ipv4 and ipv6 ports might be _different_ and collide with other running docker containers.
				HostIP:   "0.0.0.0",
				HostPort: fmt.Sprintf("%d", port)}},
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
		openPortMutex.Unlock()
		return "", nil, xerrors.Errorf("could not start resource: %w", err)
	}
	openPortMutex.Unlock()

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

// getFreePort asks the kernel for a free open port that is ready to use.
func getFreePort() (port int, err error) {
	// Binding to port 0 tells the OS to grab a port for us:
	// https://stackoverflow.com/questions/1365265/on-localhost-how-do-i-pick-a-free-port-number
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
