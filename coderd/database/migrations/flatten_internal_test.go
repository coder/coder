package migrations

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/require"
)

var migrationFileRE = regexp.MustCompile(`^(\d{6})_.+\.(?:up|down)\.sql$`)

func diskMigrations(t *testing.T) map[string]string {
	t.Helper()

	found := map[string]string{}
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Fixtures and schema dumps under testdata reuse the migration
			// naming scheme but are not migrations.
			if path == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !migrationFileRE.MatchString(d.Name()) {
			return nil
		}
		existing, ok := found[d.Name()]
		require.Falsef(t, ok, "migration %q exists at both %q and %q", d.Name(), existing, path)
		found[d.Name()] = filepath.ToSlash(path)
		return nil
	})
	require.NoError(t, err)
	return found
}

func migrationVersion(t *testing.T, name string) int {
	t.Helper()

	match := migrationFileRE.FindStringSubmatch(name)
	require.NotNilf(t, match, "not a migration file name: %q", name)
	version, err := strconv.Atoi(match[1])
	require.NoError(t, err)
	return version
}

// TestEmbeddedMigrationsCoverDisk guards a failure with no other symptom:
// golang-migrate skips directories and unparseable names silently, so an embed
// pattern that stops matching an archive directory drops migrations from the
// binary without any error.
func TestEmbeddedMigrationsCoverDisk(t *testing.T) {
	t.Parallel()

	flat, err := flatten(migrations)
	require.NoError(t, err)

	embedded := make([]string, 0, len(flat.paths))
	for name := range flat.paths {
		if migrationFileRE.MatchString(name) {
			embedded = append(embedded, name)
		}
	}

	onDisk := diskMigrations(t)
	wanted := make([]string, 0, len(onDisk))
	for name := range onDisk {
		wanted = append(wanted, name)
	}
	require.NotEmpty(t, wanted)

	slices.Sort(embedded)
	slices.Sort(wanted)
	require.Equal(t, wanted, embedded,
		"embedded migrations differ from those on disk; check the go:embed patterns in migrate.go")
}

// TestMigrationVersionsAreContiguous holds the invariant behind flattenFS: a
// deployment can be running any historical version, so none may go missing.
func TestMigrationVersionsAreContiguous(t *testing.T) {
	t.Parallel()

	ups := map[int]bool{}
	downs := map[int]bool{}
	newest := 0
	for name := range diskMigrations(t) {
		version := migrationVersion(t, name)
		if strings.HasSuffix(name, ".up.sql") {
			ups[version] = true
		} else {
			downs[version] = true
		}
		newest = max(newest, version)
	}

	require.NotZero(t, newest)
	for version := 1; version <= newest; version++ {
		require.Truef(t, ups[version], "missing up migration for version %06d", version)
		require.Truef(t, downs[version], "missing down migration for version %06d", version)
	}
}

func TestArchiveLayout(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	archives := map[string]bool{}
	archiveEnd := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		start, end, ok := parseShardDirName(entry.Name())
		if !ok {
			continue
		}
		require.Equalf(t, shardDirName(start), entry.Name(),
			"archive directory %q is not aligned to a %d-version range", entry.Name(), archiveShardSize)
		archives[entry.Name()] = true
		archiveEnd = max(archiveEnd, end)
	}

	require.NotZerof(t, archiveEnd,
		"no archive directories found; migrations 000001 onward should be archived to keep the root listable")

	for version := archiveShardSize; version <= archiveEnd; version += archiveShardSize {
		require.Truef(t, archives[shardDirName(version)],
			"archive directory %q is missing; archives must cover every version up to %06d", shardDirName(version), archiveEnd)
	}

	onDisk := diskMigrations(t)
	newest := 0
	for name := range onDisk {
		newest = max(newest, migrationVersion(t, name))
	}

	// `migrate create -seq` derives the next version from the highest migration
	// in the root directory alone. With the root emptied it restarts at 000001
	// and collides with an archived migration, so never archive every range.
	require.Lessf(t, archiveEnd, newest,
		"every migration is archived; leave the newest range in the root so new migrations keep numbering upward")

	for name, path := range onDisk {
		want := name
		if version := migrationVersion(t, name); version <= archiveEnd {
			want = shardDirName(version) + "/" + name
		}
		require.Equalf(t, want, path, "migration %q is in the wrong directory", name)
	}
}

func TestFlattenServesArchivedVersions(t *testing.T) {
	t.Parallel()

	flat, err := flatten(fstest.MapFS{
		"000001-000100/000042_archived.up.sql":   {Data: []byte("SELECT 42;")},
		"000001-000100/000042_archived.down.sql": {Data: []byte("SELECT -42;")},
		"000501_recent.up.sql":                   {Data: []byte("SELECT 501;")},
		"000501_recent.down.sql":                 {Data: []byte("SELECT -501;")},
	})
	require.NoError(t, err)

	driver, err := iofs.New(flat, ".")
	require.NoError(t, err)
	t.Cleanup(func() { _ = driver.Close() })

	first, err := driver.First()
	require.NoError(t, err)
	require.EqualValues(t, 42, first)

	body, _, err := driver.ReadUp(42)
	require.NoError(t, err)
	defer body.Close()
	content, err := io.ReadAll(body)
	require.NoError(t, err)
	require.Equal(t, "SELECT 42;", string(content))

	next, err := driver.Next(42)
	require.NoError(t, err)
	require.EqualValues(t, 501, next)
}

func TestFlattenIsConformingFS(t *testing.T) {
	t.Parallel()

	flat, err := flatten(fstest.MapFS{
		"000001-000100/000042_archived.up.sql": {Data: []byte("SELECT 42;")},
		"000501_recent.up.sql":                 {Data: []byte("SELECT 501;")},
	})
	require.NoError(t, err)
	require.NoError(t, fstest.TestFS(flat, "000042_archived.up.sql", "000501_recent.up.sql"))

	// The flattened view must not expose the archive directories themselves, or
	// walking it would yield each migration twice.
	_, err = flat.Open("000001-000100")
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestFlattenClosesInner(t *testing.T) {
	t.Parallel()

	t.Run("Closable", func(t *testing.T) {
		t.Parallel()

		inner := &closableFS{FS: fstest.MapFS{"000501_recent.up.sql": {}}}
		flat, err := flatten(inner)
		require.NoError(t, err)

		driver, err := iofs.New(flat, ".")
		require.NoError(t, err)
		require.NoError(t, driver.Close())
		require.True(t, inner.closed)
	})

	t.Run("NotClosable", func(t *testing.T) {
		t.Parallel()

		flat, err := flatten(fstest.MapFS{"000501_recent.up.sql": {}})
		require.NoError(t, err)

		driver, err := iofs.New(flat, ".")
		require.NoError(t, err)
		require.NoError(t, driver.Close())
	})
}

type closableFS struct {
	fs.FS
	closed bool
}

func (c *closableFS) Close() error {
	c.closed = true
	return nil
}

func TestFlattenRejectsDuplicateMigrations(t *testing.T) {
	t.Parallel()

	_, err := flatten(fstest.MapFS{
		"000042_dupe.up.sql":               {},
		"000001-000100/000042_dupe.up.sql": {},
	})
	require.ErrorContains(t, err, "duplicate migration")
}

func TestFlattenIgnoresNonArchiveDirectories(t *testing.T) {
	t.Parallel()

	flat, err := flatten(fstest.MapFS{
		"000501_recent.up.sql":                 {},
		"000001-000100/000042_archived.up.sql": {},
		"testdata/fixtures/000042_fixture.sql": {},
		"full_dumps/v0.12.2/schema.sql":        {},
	})
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"000501_recent.up.sql":   "000501_recent.up.sql",
		"000042_archived.up.sql": "000001-000100/000042_archived.up.sql",
	}, flat.paths)
}

func TestParseShardDirName(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		start int
		end   int
		ok    bool
	}{
		{name: "000001-000100", start: 1, end: 100, ok: true},
		{name: "000501-000600", start: 501, end: 600, ok: true},
		{name: "000100-000001"},
		{name: "0001-0100"},
		{name: "000001-abcdef"},
		{name: "-000100"},
		{name: "000001"},
		{name: "testdata"},
		{name: "+00001-000100"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			start, end, ok := parseShardDirName(tc.name)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.start, start)
			require.Equal(t, tc.end, end)
		})
	}
}
