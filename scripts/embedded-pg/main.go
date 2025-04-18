// Start an embedded postgres database on port 5432. Used in CI on macOS and Windows.
package main

import (
	"database/sql"
	"flag"
	"os"
	"path/filepath"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

func main() {
	var customPath string
	flag.StringVar(&customPath, "path", "", "Optional custom path for postgres data directory")
	flag.Parse()

	postgresPath := filepath.Join(os.TempDir(), "coder-test-postgres")
	if customPath != "" {
		postgresPath = customPath
	}

	ep := embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().
			Version(embeddedpostgres.V16).
			BinariesPath(filepath.Join(postgresPath, "bin")).
			// Default BinaryRepositoryURL repo1.maven.org is flaky.
			BinaryRepositoryURL("https://repo.maven.apache.org/maven2").
			DataPath(filepath.Join(postgresPath, "data")).
			RuntimePath(filepath.Join(postgresPath, "runtime")).
			CachePath(filepath.Join(postgresPath, "cache")).
			Username("postgres").
			Password("postgres").
			Database("postgres").
			Encoding("UTF8").
			Port(uint32(5432)).
			Logger(os.Stdout),
	)
	err := ep.Start()
	if err != nil {
		panic(err)
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
		panic(err)
	}
	for _, query := range paramQueries {
		if _, err := db.Exec(query); err != nil {
			panic(err)
		}
	}
	if err := db.Close(); err != nil {
		panic(err)
	}
	// We restart the database to apply all the parameters.
	if err := ep.Stop(); err != nil {
		panic(err)
	}
	if err := ep.Start(); err != nil {
		panic(err)
	}
}
