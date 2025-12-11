// Start an embedded postgres database on port 5432. Used in CI on macOS and Windows.
package main

import (
	"database/sql"
	"flag"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

func main() {
	var customPath string
	var cachePath string
	flag.StringVar(&customPath, "path", "", "Optional custom path for postgres data directory")
	flag.StringVar(&cachePath, "cache", "", "Optional custom path for embedded postgres binaries")
	flag.Parse()

	postgresPath := filepath.Join(os.TempDir(), "coder-test-postgres")
	if customPath != "" {
		postgresPath = customPath
	}
	if err := os.MkdirAll(postgresPath, os.ModePerm); err != nil {
		log.Fatalf("Failed to create directory %s: %v", postgresPath, err)
	}
	if cachePath == "" {
		cachePath = filepath.Join(postgresPath, "cache")
	}
	if err := os.MkdirAll(cachePath, os.ModePerm); err != nil {
		log.Fatalf("Failed to create directory %s: %v", cachePath, err)
	}

	ep := embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().
			Version(embeddedpostgres.V16).
			BinariesPath(filepath.Join(postgresPath, "bin")).
			BinaryRepositoryURL("https://repo.maven.apache.org/maven2").
			DataPath(filepath.Join(postgresPath, "data")).
			RuntimePath(filepath.Join(postgresPath, "runtime")).
			CachePath(cachePath).
			Username("postgres").
			Password("postgres").
			Database("postgres").
			Encoding("UTF8").
			Port(uint32(5432)).
			Logger(os.Stdout),
	)
	err := ep.Start()
	if err != nil {
		log.Fatalf("Failed to start embedded postgres: %v", err)
	}

	// Troubleshooting: list files in cachePath
	if err := filepath.Walk(cachePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		switch {
		case info.IsDir():
			log.Printf("D: %s", path)
		case info.Mode().IsRegular():
			log.Printf("F: %s [%s] (%d bytes) %s", path, info.Mode().String(), info.Size(), info.ModTime().Format(time.RFC3339))
		default:
			log.Printf("Other: %s [%s] %s", path, info.Mode(), info.ModTime().Format(time.RFC3339))
		}
		return nil
	}); err != nil {
		log.Printf("Failed to list files in cachePath %s: %v", cachePath, err)
	}

	// We execute these queries instead of using the embeddedpostgres
	// StartParams because it doesn't work on Windows. The library
	// seems to have a bug where it sends malformed parameters to
	// pg_ctl. It encloses each parameter in single quotes, which
	// Windows can't handle.
	// Related issue:
	// https://github.com/fergusstrange/embedded-postgres/issues/145
	//
	// Optimized for CI: speed over durability, crash safety disabled.
	paramQueries := []string{
		// Disable durability, data doesn't need to survive a crash.
		`ALTER SYSTEM SET fsync = 'off';`,
		`ALTER SYSTEM SET synchronous_commit = 'off';`,
		`ALTER SYSTEM SET full_page_writes = 'off';`,
		`ALTER SYSTEM SET wal_level = 'minimal';`,
		`ALTER SYSTEM SET max_wal_senders = '0';`,

		// Minimize disk writes by batching and avoiding flushes.
		`ALTER SYSTEM SET checkpoint_timeout = '30min';`,
		`ALTER SYSTEM SET max_wal_size = '4GB';`,
		`ALTER SYSTEM SET min_wal_size = '1GB';`,
		`ALTER SYSTEM SET backend_flush_after = '0';`,
		`ALTER SYSTEM SET checkpoint_flush_after = '0';`,
		`ALTER SYSTEM SET wal_buffers = '64MB';`,
		`ALTER SYSTEM SET bgwriter_lru_maxpages = '0';`,

		// Tests are short-lived and each uses its own database.
		`ALTER SYSTEM SET autovacuum = 'off';`,
		`ALTER SYSTEM SET track_activities = 'off';`,
		`ALTER SYSTEM SET track_counts = 'off';`,

		// Reduce overhead from JIT and logging.
		`ALTER SYSTEM SET jit = 'off';`,
		`ALTER SYSTEM SET log_checkpoints = 'off';`,
		`ALTER SYSTEM SET log_connections = 'off';`,
		`ALTER SYSTEM SET log_disconnections = 'off';`,

		`ALTER SYSTEM SET max_connections = '1000';`,
		`ALTER SYSTEM SET client_encoding = 'UTF8';`,
	}

	// Memory and I/O settings tuned per Depot runner specs:
	// - macOS: 24GB RAM, fast disk (has disk accelerator)
	// - Windows: 128GB RAM, slow disk (EBS, no accelerator)
	switch runtime.GOOS {
	case "darwin":
		paramQueries = append(paramQueries,
			`ALTER SYSTEM SET shared_buffers = '4GB';`,
			`ALTER SYSTEM SET effective_cache_size = '8GB';`,
			`ALTER SYSTEM SET work_mem = '32MB';`,
			`ALTER SYSTEM SET maintenance_work_mem = '512MB';`,
			`ALTER SYSTEM SET temp_buffers = '64MB';`,
			`ALTER SYSTEM SET random_page_cost = '1.0';`,
		)
	case "windows":
		paramQueries = append(paramQueries,
			`ALTER SYSTEM SET shared_buffers = '8GB';`,
			`ALTER SYSTEM SET effective_cache_size = '32GB';`,
			`ALTER SYSTEM SET work_mem = '64MB';`,
			`ALTER SYSTEM SET maintenance_work_mem = '1GB';`,
			`ALTER SYSTEM SET temp_buffers = '128MB';`,
			`ALTER SYSTEM SET random_page_cost = '4.0';`,
		)
	}
	db, err := sql.Open("postgres", "postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable")
	if err != nil {
		log.Fatalf("Failed to connect to embedded postgres: %v", err)
	}
	for _, query := range paramQueries {
		if _, err := db.Exec(query); err != nil {
			log.Fatalf("Failed to execute setup query %q: %v", query, err)
		}
	}
	if err := db.Close(); err != nil {
		log.Fatalf("Failed to close database connection: %v", err)
	}
	// We restart the database to apply all the parameters.
	if err := ep.Stop(); err != nil {
		log.Fatalf("Failed to stop embedded postgres after applying parameters: %v", err)
	}
	if err := ep.Start(); err != nil {
		log.Fatalf("Failed to start embedded postgres after applying parameters: %v", err)
	}
}
