package coderd

import (
	"context"
	"database/sql"
	"io/fs"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/files"
	"github.com/coder/coder/v2/coderd/util/ptr"
)

func Test_prepareDynamicPreview(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		db, fc, ver, user := setupPrepareDynamicPreview(t, ``, ptr.Ref("{}"))
		rec := httptest.NewRecorder()
		_, closer, success := prepareDynamicPreview(context.Background(), rec, db, fc, ver, user)
		require.True(t, success)

		require.Equal(t, fc.Count(), 3)
		closer()

		require.Equal(t, fc.Count(), 0)
	})
}

func setupPrepareDynamicPreview(t *testing.T, tf string, modules *string) (database.Store, *files.Cache, database.TemplateVersion, database.User) {
	db := dbmem.New()

	versionFile := dbgen.File(t, db, database.File{})

	user := dbgen.User(t, db, database.User{})
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		FileID: versionFile.ID,
	})

	ver := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		JobID: job.ID,
	})

	modulesFile := uuid.NullUUID{}
	if modules != nil {
		modulesFile = uuid.NullUUID{
			UUID:  uuid.New(),
			Valid: true,
		}
	}

	dbgen.TemplateVersionTerraformValues(t, db, database.InsertTemplateVersionTerraformValuesByJobIDParams{
		JobID:             job.ID,
		UpdatedAt:         time.Time{},
		CachedPlan:        nil,
		CachedModuleFiles: modulesFile,
	})

	fc := files.New(func(ctx context.Context, u uuid.UUID) (fs.FS, error) {
		mem := afero.NewMemMapFs()

		if u == versionFile.ID {
			f, err := mem.Create("main.tf")
			if err != nil {
				return nil, xerrors.Errorf("create file: %w", err)
			}
			_, _ = f.WriteString(tf)
			_ = f.Close()
			return afero.NewIOFS(mem), nil
		}

		if modulesFile.Valid && u == modulesFile.UUID {
			f, err := mem.Create("modules.json")
			if err != nil {
				return nil, xerrors.Errorf("create file: %w", err)
			}
			_, _ = f.WriteString(*modules)
			_ = f.Close()
			return afero.NewIOFS(mem), nil
		}

		return nil, sql.ErrNoRows
	})

	return db, fc, ver, user
}
