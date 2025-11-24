package dbtestutil

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cryptorand"
)

const CoderTestingDBName = "coder_testing"

//go:embed coder_testing.sql
var coderTestingSQLInit string

type Broker struct {
	sync.Mutex
	uuid           uuid.UUID
	coderTestingDB *sql.DB
	refCount       int
	// we keep a reference to the stdin of the cleaner so that Go doesn't garbage collect it.
	cleanerFD any
}

func (b *Broker) Create(t TBSubset, opts ...OpenOption) (ConnectionParams, error) {
	if err := b.init(t); err != nil {
		return ConnectionParams{}, err
	}
	openOptions := OpenOptions{}
	for _, opt := range opts {
		opt(&openOptions)
	}

	var (
		username = defaultConnectionParams.Username
		password = defaultConnectionParams.Password
		host     = defaultConnectionParams.Host
		port     = defaultConnectionParams.Port
	)
	packageName := getTestPackageName(t)
	testName := t.Name()

	// Use a time-based prefix to make it easier to find the database
	// when debugging.
	now := time.Now().Format("test_2006_01_02_15_04_05")
	dbSuffix, err := cryptorand.StringCharset(cryptorand.Lower, 10)
	if err != nil {
		return ConnectionParams{}, xerrors.Errorf("generate db suffix: %w", err)
	}
	dbName := now + "_" + dbSuffix

	_, err = b.coderTestingDB.Exec(
		"INSERT INTO test_databases (name, process_uuid, test_package, test_name) VALUES ($1, $2, $3, $4)",
		dbName, b.uuid, packageName, testName)
	if err != nil {
		return ConnectionParams{}, xerrors.Errorf("insert test_database row: %w", err)
	}

	// if empty createDatabaseFromTemplate will create a new template db
	templateDBName := os.Getenv("DB_FROM")
	if openOptions.DBFrom != nil {
		templateDBName = *openOptions.DBFrom
	}
	if err = createDatabaseFromTemplate(t, defaultConnectionParams, b.coderTestingDB, dbName, templateDBName); err != nil {
		return ConnectionParams{}, xerrors.Errorf("create database: %w", err)
	}

	testDBParams := ConnectionParams{
		Username: username,
		Password: password,
		Host:     host,
		Port:     port,
		DBName:   dbName,
	}

	// Optionally log the DSN to help connect to the test database.
	if openOptions.LogDSN {
		_, _ = fmt.Fprintf(os.Stderr, "Connect to the database for %s using: psql '%s'\n", t.Name(), testDBParams.DSN())
	}
	t.Cleanup(b.clean(t, dbName))
	return testDBParams, nil
}

func (b *Broker) clean(t TBSubset, dbName string) func() {
	return func() {
		_, err := b.coderTestingDB.Exec("DROP DATABASE " + dbName + ";")
		if err != nil {
			t.Logf("failed to clean up database %q: %s\n", dbName, err.Error())
			return
		}
		_, err = b.coderTestingDB.Exec("UPDATE test_databases SET dropped_at = CURRENT_TIMESTAMP WHERE name = $1", dbName)
		if err != nil {
			t.Logf("failed to mark test database '%s' dropped: %s\n", dbName, err.Error())
		}
	}
}

func (b *Broker) init(t TBSubset) error {
	b.Lock()
	defer b.Unlock()
	if b.coderTestingDB != nil {
		// already initialized
		b.refCount++
		t.Cleanup(b.decRef)
		return nil
	}

	connectionParamsInitOnce.Do(func() {
		errDefaultConnectionParamsInit = initDefaultConnection(t)
	})
	if errDefaultConnectionParamsInit != nil {
		return xerrors.Errorf("init default connection params: %w", errDefaultConnectionParamsInit)
	}
	coderTestingParams := defaultConnectionParams
	coderTestingParams.DBName = CoderTestingDBName
	coderTestingDB, err := sql.Open("postgres", coderTestingParams.DSN())
	if err != nil {
		return xerrors.Errorf("open postgres connection: %w", err)
	}

	// coderTestingSQLInit is idempotent, so we can run it every time.
	_, err = coderTestingDB.Exec(coderTestingSQLInit)
	var pqErr *pq.Error
	if xerrors.As(err, &pqErr) && pqErr.Code == "3D000" {
		// database does not exist.
		if closeErr := coderTestingDB.Close(); closeErr != nil {
			return xerrors.Errorf("close postgres connection: %w", closeErr)
		}
		err = createCoderTestingDB(t)
		if err != nil {
			return xerrors.Errorf("create coder testing db: %w", err)
		}
		coderTestingDB, err = sql.Open("postgres", coderTestingParams.DSN())
		if err != nil {
			return xerrors.Errorf("open postgres connection: %w", err)
		}
	} else if err != nil {
		_ = coderTestingDB.Close()
		return xerrors.Errorf("ping '%s' database: %w", CoderTestingDBName, err)
	}
	b.coderTestingDB = coderTestingDB
	b.refCount++
	t.Cleanup(b.decRef)

	if b.uuid == uuid.Nil {
		b.uuid = uuid.New()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		b.cleanerFD, err = startCleaner(ctx, t, b.uuid, coderTestingParams.DSN())
		if err != nil {
			return xerrors.Errorf("start test db cleaner: %w", err)
		}
	}
	return nil
}

func createCoderTestingDB(t TBSubset) error {
	db, err := sql.Open("postgres", defaultConnectionParams.DSN())
	if err != nil {
		return xerrors.Errorf("open postgres connection: %w", err)
	}
	defer func() {
		_ = db.Close()
	}()
	err = createAndInitDatabase(t, defaultConnectionParams, db, CoderTestingDBName, func(testDB *sql.DB) error {
		_, err := testDB.Exec(coderTestingSQLInit)
		return err
	})
	if err != nil {
		return xerrors.Errorf("create coder testing db: %w", err)
	}
	return nil
}

func (b *Broker) decRef() {
	b.Lock()
	defer b.Unlock()
	b.refCount--
	if b.refCount == 0 {
		// ensures we don't leave go routines around for GoLeak to find.
		_ = b.coderTestingDB.Close()
		b.coderTestingDB = nil
	}
}

// getTestPackageName returns the package name of the test that called it.
func getTestPackageName(t TBSubset) string {
	packageName := "unknown"
	// Ask runtime.Callers for up to 100 program counters, including runtime.Callers itself.
	pc := make([]uintptr, 100)
	n := runtime.Callers(0, pc)
	if n == 0 {
		// No PCs available. This can happen if the first argument to
		// runtime.Callers is large.
		//
		// Return now to avoid processing the zero Frame that would
		// otherwise be returned by frames.Next below.
		t.Logf("could not determine test package name: no PCs available")
		return packageName
	}

	pc = pc[:n] // pass only valid pcs to runtime.CallersFrames
	frames := runtime.CallersFrames(pc)

	// Loop to get frames.
	// A fixed number of PCs can expand to an indefinite number of Frames.
	for {
		frame, more := frames.Next()

		if strings.HasPrefix(frame.Function, "github.com/coder/coder/v2/") {
			packageName = strings.SplitN(strings.TrimPrefix(frame.Function, "github.com/coder/coder/v2/"), ".", 2)[0]
		}
		if strings.HasPrefix(frame.Function, "testing") {
			break
		}

		// Check whether there are more frames to process after this one.
		if !more {
			break
		}
	}
	return packageName
}
