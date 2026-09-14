// Start an embedded postgres database on port 5432. Used in CI on macOS and Windows.
package main

import (
	"database/sql"
	"flag"
	"log"
	"os"
	"path/filepath"
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
	paramQueries := []string{
		`ALTER SYSTEM SET effective_cache_size = '1GB';`,
		`ALTER SYSTEM SET fsync = 'off';`,
		`ALTER SYSTEM SET full_page_writes = 'off';`,
		`ALTER SYSTEM SET max_connections = '1000';`,
		`ALTER SYSTEM SET shared_buffers = '1GB';`,
		`ALTER SYSTEM SET synchronous_commit = 'off';`,
		`ALTER SYSTEM SET client_encoding = 'UTF8';`,
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
