package dbcrypt_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/enterprise/dbcrypt"
	"github.com/coder/coder/v2/testutil"
)

func TestGitHubRunnerTokenEncryption(t *testing.T) {
	t.Parallel()
	f := newRotateFixture(t)
	org := dbgen.Organization(t, f.rawDB, database.Organization{})
	user := dbgen.User(t, f.rawDB, database.User{})
	template := dbgen.Template(t, f.rawDB, database.Template{OrganizationID: org.ID, CreatedBy: user.ID})
	pool, err := f.cryptDBA.InsertGitHubRunnerPool(f.ctx, database.InsertGitHubRunnerPoolParams{ID: uuid.New(), OrganizationID: org.ID, OwnerID: user.ID, TemplateID: template.ID, Name: "test", GithubConfigUrl: "https://github.com/example/repo", GithubToken: "private-token", RunnerGroupID: 1, MaxRunners: 1, StartupTimeoutSeconds: 600, JobTimeoutSeconds: 3600})
	require.NoError(t, err)
	require.Equal(t, "private-token", pool.GithubToken)
	raw, err := f.rawDB.GetGitHubRunnerPoolByID(f.ctx, pool.ID)
	require.NoError(t, err)
	require.NotEqual(t, "private-token", raw.GithubToken)
	require.Equal(t, f.cipherA.HexDigest(), raw.GithubTokenKeyID.String)
	f.rotate(t)
	raw, err = f.rawDB.GetGitHubRunnerPoolByID(f.ctx, pool.ID)
	require.NoError(t, err)
	require.Equal(t, f.cipherB.HexDigest(), raw.GithubTokenKeyID.String)
	cryptB, err := dbcrypt.New(f.ctx, f.rawDB, f.cipherB)
	require.NoError(t, err)
	decrypted, err := cryptB.GetGitHubRunnerPoolByID(f.ctx, pool.ID)
	require.NoError(t, err)
	require.Equal(t, "private-token", decrypted.GithubToken)
	require.NoError(t, dbcrypt.Decrypt(f.ctx, testutil.Logger(t), f.sqlDB, []dbcrypt.Cipher{f.cipherB}))
	raw, err = f.rawDB.GetGitHubRunnerPoolByID(f.ctx, pool.ID)
	require.NoError(t, err)
	require.Equal(t, "private-token", raw.GithubToken)
	require.False(t, raw.GithubTokenKeyID.Valid)
}
