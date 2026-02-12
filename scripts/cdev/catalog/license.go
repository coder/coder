package catalog

import (
	"context"
	"database/sql"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"

	_ "github.com/lib/pq"
)

// RequireLicense panics if CODER_LICENSE is not set. Call this
// during the configuration phase for features that require a
// license (external provisioners, HA).
func RequireLicense(feature string) {
	if os.Getenv("CODER_LICENSE") == "" {
		panic("CODER_LICENSE must be set when using " + feature)
	}
}

// waitForMigrations polls the schema_migrations table until
// migrations are complete (version != 0 and dirty = false).
// This is necessary because EnsureLicense may run concurrently
// with coderd's startup, which performs migrations.
func waitForMigrations(ctx context.Context, logger slog.Logger, db *sql.DB) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	timeout := time.After(5 * time.Minute)

	for {
		var version int64
		var dirty bool
		err := db.QueryRowContext(ctx,
			"SELECT version, dirty FROM schema_migrations LIMIT 1",
		).Scan(&version, &dirty)
		if err == nil && version != 0 && !dirty {
			logger.Info(ctx, "migrations complete",
				slog.F("version", version),
			)
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return xerrors.New("timed out waiting for migrations")
		case <-ticker.C:
		}
	}
}

// EnsureLicense checks if the license JWT from CODER_LICENSE is
// already in the database, and inserts it if not. The JWT is parsed
// without verification to extract the exp and uuid claims — this is
// acceptable since cdev is a development tool.
func EnsureLicense(ctx context.Context, logger slog.Logger, pgURL string) error {
	licenseJWT := os.Getenv("CODER_LICENSE")
	if licenseJWT == "" {
		return nil
	}

	sqlDB, err := sql.Open("postgres", pgURL)
	if err != nil {
		return xerrors.Errorf("open database: %w", err)
	}
	defer sqlDB.Close()

	// Wait for coderd to finish running migrations before
	// attempting to read or write the licenses table.
	if err := waitForMigrations(ctx, logger, sqlDB); err != nil {
		return xerrors.Errorf("wait for migrations: %w", err)
	}

	store := database.New(sqlDB)

	// Check if this exact JWT is already in the database.
	licenses, err := store.GetLicenses(ctx)
	if err != nil {
		return xerrors.Errorf("get licenses: %w", err)
	}
	for _, lic := range licenses {
		if lic.JWT == licenseJWT {
			logger.Info(ctx, "license already present in database")
			return nil
		}
	}

	// Parse JWT claims without verification to extract exp and uuid.
	parser := jwt.NewParser()
	claims := &jwt.RegisteredClaims{}
	_, _, err = parser.ParseUnverified(licenseJWT, claims)
	if err != nil {
		return xerrors.Errorf("parse license JWT: %w", err)
	}

	if claims.ExpiresAt == nil {
		return xerrors.New("license JWT missing exp claim")
	}

	// UUID comes from the standard "jti" claim (claims.ID).
	// Fallback to random UUID for older licenses without one.
	licenseUUID, err := uuid.Parse(claims.ID)
	if err != nil {
		licenseUUID = uuid.New()
	}

	_, err = store.InsertLicense(ctx, database.InsertLicenseParams{
		UploadedAt: dbtime.Now(),
		JWT:        licenseJWT,
		Exp:        claims.ExpiresAt.Time,
		UUID:       licenseUUID,
	})
	if err != nil {
		return xerrors.Errorf("insert license: %w", err)
	}

	logger.Info(ctx, "license inserted into database",
		slog.F("uuid", licenseUUID),
		slog.F("expires", claims.ExpiresAt.Time),
	)
	return nil
}
