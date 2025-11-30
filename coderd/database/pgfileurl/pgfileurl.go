// Package pgfileurl provides a SQL driver that reads the database connection
// URL from a file on each connection attempt. This supports credential rotation
// in environments like Kubernetes where secrets may be updated without
// restarting the application.
package pgfileurl

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"hash/fnv"
	"os"
	"strings"
	"sync"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

// hashString returns a simple hash of the string for use in driver names.
func hashString(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

// driver name registry to avoid duplicate registration.
var (
	registryMu sync.Mutex
	registry   = make(map[string]bool)
)

type fileURLDriver struct {
	parent   driver.Driver
	filePath string
	logger   slog.Logger
}

var _ driver.Driver = &fileURLDriver{}

// Register creates and registers a SQL driver that reads the connection URL
// from a file on each connection attempt. This supports credential rotation
// in environments like Kubernetes where secrets may be updated.
//
// The returned driver name should be used with sql.Open(). The dsn parameter
// to sql.Open() is ignored since the URL is read from the file.
//
// The logger parameter is used for debug logging when reading the file.
func Register(parentName, filePath string, logger slog.Logger) (string, error) {
	db, err := sql.Open(parentName, "")
	if err != nil {
		return "", xerrors.Errorf("open parent driver: %w", err)
	}
	defer db.Close()

	d := &fileURLDriver{
		parent:   db.Driver(),
		filePath: filePath,
		logger:   logger,
	}

	// Include a hash of the file path in the driver name to ensure uniqueness
	// when multiple drivers are registered with different file paths (e.g., in tests).
	name := fmt.Sprintf("%s-fileurl-%d", parentName, hashString(filePath))

	registryMu.Lock()
	defer registryMu.Unlock()
	if !registry[name] {
		sql.Register(name, d)
		registry[name] = true
	}

	return name, nil
}

// Open reads the connection URL from the file and opens a connection.
// The name parameter is ignored; the URL is always read from the file
// specified during Register().
func (d *fileURLDriver) Open(_ string) (driver.Conn, error) {
	// Read fresh URL from file on every connection attempt.
	content, err := os.ReadFile(d.filePath)
	if err != nil {
		return nil, xerrors.Errorf("read database URL file %q: %w", d.filePath, err)
	}
	dbURL := strings.TrimSpace(string(content))

	d.logger.Debug(context.Background(), "re-reading database connection URL from file")

	conn, err := d.parent.Open(dbURL)
	if err != nil {
		return nil, xerrors.Errorf("open connection: %w", err)
	}

	return conn, nil
}
