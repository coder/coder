package main

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"regexp"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/migrations"
)

// This script validates the migration path between two versions.
// It performs the following actions:
// Given OLD_VERSION and NEW_VERSION:
//  1. Checks out $OLD_VERSION and inits schema at that version.
//  2. Checks out $NEW_VERSION and runs migrations.
//  3. Compares database schema post-migrate to that in VCS.
//     If any diffs are found, exits with an error.
func main() {
	var (
		migrateFromVersion string
		migrateToVersion   string
		postgresURL        string
		skipCleanup        bool
	)

	flag.StringVar(&migrateFromVersion, "from", "", "Migrate from this version")
	flag.StringVar(&migrateToVersion, "to", "", "Migrate to this version")
	flag.StringVar(&postgresURL, "postgres-url", "postgresql://postgres:postgres@localhost:5432/postgres?sslmode=disable", "Postgres URL to migrate")
	flag.BoolVar(&skipCleanup, "skip-cleanup", false, "Do not clean up on exit.")
	flag.Parse()

	if migrateFromVersion == "" || migrateToVersion == "" {
		_, _ = fmt.Fprintln(os.Stderr, "must specify --from=<old version> and --to=<new version>")
		os.Exit(1)
	}

	_, _ = fmt.Fprintf(os.Stderr, "Read schema at version %q\n", migrateToVersion)
	expectedSchemaAfter, err := gitShow("coderd/database/dump.sql", migrateToVersion)
	if err != nil {
		panic(err)
	}

	_, _ = fmt.Fprintf(os.Stderr, "Read migrations for %q\n", migrateFromVersion)
	migrateFromFS, err := makeMigrateFS(migrateFromVersion)
	if err != nil {
		panic(err)
	}
	_, _ = fmt.Fprintf(os.Stderr, "Read migrations for %q\n", migrateToVersion)
	migrateToFS, err := makeMigrateFS(migrateToVersion)
	if err != nil {
		panic(err)
	}

	_, _ = fmt.Fprintf(os.Stderr, "Connect to postgres\n")
	conn, err := sql.Open("postgres", postgresURL)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	ver, err := checkMigrateVersion(conn)
	if err != nil {
		panic(err)
	}
	if ver < 0 {
		_, _ = fmt.Fprintf(os.Stderr, "No previous migration detected.\n")
	} else {
		_, _ = fmt.Fprintf(os.Stderr, "Detected migration version %d\n", ver)
	}

	_, _ = fmt.Fprintf(os.Stderr, "Init database at version %q\n", migrateFromVersion)
	if err := migrations.UpWithFS(conn, migrateFromFS); err != nil {
		panic(err)
	}

	_, _ = fmt.Fprintf(os.Stderr, "Migrate to version %q\n", migrateToVersion)
	if err := migrations.UpWithFS(conn, migrateToFS); err != nil {
		panic(err)
	}

	_, _ = fmt.Fprintf(os.Stderr, "Dump schema at version %q\n", migrateToVersion)
	dumpBytesAfter, err := dbtestutil.PGDumpSchemaOnly(postgresURL)
	if err != nil {
		panic(err)
	}

	if diff := cmp.Diff(string(dumpBytesAfter), string(stripGenPreamble(expectedSchemaAfter))); diff != "" {
		_, _ = fmt.Fprintf(os.Stderr, "Schema differs from expected after migration: %s\n", diff)
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(os.Stderr, "OK\n")
}

func makeMigrateFS(version string) (fs.FS, error) {
	// Export the migrations from the requested version to a zip archive
	out, err := exec.Command("git", "archive", "--format=zip", version, "coderd/database/migrations").CombinedOutput()
	if err != nil {
		return nil, xerrors.Errorf("git archive: %s\n", out)
	}
	// Make a zip.Reader on top of it. This implements fs.fs!
	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		return nil, xerrors.Errorf("create zip reader: %w", err)
	}
	// Sub-FS to it's rooted at migrations dir.
	subbed, err := fs.Sub(zr, "coderd/database/migrations")
	if err != nil {
		return nil, xerrors.Errorf("sub fs: %w", err)
	}
	return subbed, nil
}

func gitShow(path, version string) ([]byte, error) {
	out, err := exec.Command("git", "show", version+":"+path).CombinedOutput() //nolint:gosec
	if err != nil {
		return nil, xerrors.Errorf("git show: %s\n", out)
	}
	return out, nil
}

func stripGenPreamble(bs []byte) []byte {
	return regexp.MustCompile(`(?im)^(-- Code generated.*DO NOT EDIT.)$`).ReplaceAll(bs, []byte{})
}

func checkMigrateVersion(conn *sql.DB) (int, error) {
	var version int
	rows, err := conn.Query(`SELECT version FROM schema_migrations LIMIT 1;`)
	if err != nil {
		return -1, nil // not migrated
	}
	for rows.Next() {
		if err := rows.Scan(&version); err != nil {
			return 0, xerrors.Errorf("scan version: %w", err)
		}
	}
	return version, nil
}
