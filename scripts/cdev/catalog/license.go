package catalog

import (
	"context"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"

	_ "github.com/lib/pq" // Imported for postgres driver side effects.
)

// RequireLicense panics if CODER_LICENSE is not set. Call this
// during the configuration phase for features that require a
// license (external provisioners, HA).
func RequireLicense(feature string) {
	if os.Getenv("CODER_LICENSE") == "" {
		panic("CODER_LICENSE must be set when using " + feature)
	}
}

// EnsureLicense checks if the license JWT from CODER_LICENSE is
// already in the database, and inserts it if not. The JWT is parsed
// without verification to extract the exp and uuid claims — this is
// acceptable since cdev is a development tool.
func EnsureLicense(ctx context.Context, logger slog.Logger, cat *Catalog) error {
	licenseJWT := os.Getenv("CODER_LICENSE")
	if licenseJWT == "" {
		return nil
	}

	pg, ok := cat.MustGet(OnPostgres()).(*Postgres)
	if !ok {
		return xerrors.New("unexpected type for Postgres service")
	}

	// Wait for coderd to finish running migrations before
	// attempting to read or write the licenses table.
	beforeMig := time.Now()
	err := pg.waitForMigrations(ctx, logger)
	if err != nil {
		return xerrors.Errorf("wait for postgres migrations: %w", err)
	}
	logger.Info(ctx, "waited for postgres migrations", slog.F("duration", time.Since(beforeMig)))

	db, err := pg.sqlDB()
	if err != nil {
		return xerrors.Errorf("connect to database: %w", err)
	}
	defer db.Close()
	store := database.New(db)

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
		slog.F("license_id", licenseUUID),
		slog.F("expires", claims.ExpiresAt.Time),
	)
	return nil
}
