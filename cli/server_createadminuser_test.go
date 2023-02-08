package cli_test

import (
	"context"
	"database/sql"
	"fmt"
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/postgres"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/userpassword"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/testutil"
)

//nolint:paralleltest, tparallel
func TestServerCreateAdminUser(t *testing.T) {
	const (
		username = "dean"
		email    = "dean@example.com"
		password = "SecurePa$$word123"
	)

	verifyUser := func(t *testing.T, dbURL, username, email, password string) {
		t.Helper()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		sqlDB, err := sql.Open("postgres", dbURL)
		require.NoError(t, err)
		defer sqlDB.Close()
		db := database.New(sqlDB)

		pingCtx, pingCancel := context.WithTimeout(ctx, testutil.WaitShort)
		defer pingCancel()
		_, err = db.Ping(pingCtx)
		require.NoError(t, err, "ping db")

		user, err := db.GetUserByEmailOrUsername(ctx, database.GetUserByEmailOrUsernameParams{
			Email: email,
		})
		require.NoError(t, err)
		require.Equal(t, username, user.Username, "username does not match")
		require.Equal(t, email, user.Email, "email does not match")

		ok, err := userpassword.Compare(string(user.HashedPassword), password)
		require.NoError(t, err)
		require.True(t, ok, "password does not match")

		require.EqualValues(t, []string{rbac.RoleOwner()}, user.RBACRoles, "user does not have owner role")

		// Check that user is admin in every org.
		orgs, err := db.GetOrganizations(ctx)
		require.NoError(t, err)
		orgIDs := make(map[uuid.UUID]struct{}, len(orgs))
		for _, org := range orgs {
			orgIDs[org.ID] = struct{}{}
		}

		orgMemberships, err := db.GetOrganizationMembershipsByUserID(ctx, user.ID)
		require.NoError(t, err)
		orgIDs2 := make(map[uuid.UUID]struct{}, len(orgMemberships))
		for _, membership := range orgMemberships {
			orgIDs2[membership.OrganizationID] = struct{}{}
			assert.Equal(t, []string{rbac.RoleOrgAdmin(membership.OrganizationID)}, membership.Roles, "user is not org admin")
		}

		require.Equal(t, orgIDs, orgIDs2, "user is not in all orgs")
	}

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS != "linux" || testing.Short() {
			// Skip on non-Linux because it spawns a PostgreSQL instance.
			t.SkipNow()
		}
		connectionURL, closeFunc, err := postgres.Open()
		require.NoError(t, err)
		defer closeFunc()

		sqlDB, err := sql.Open("postgres", connectionURL)
		require.NoError(t, err)
		defer sqlDB.Close()
		db := database.New(sqlDB)

		// Sometimes generating SSH keys takes a really long time if there isn't
		// enough entropy. We don't want the tests to fail in these cases.
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
		defer cancel()

		pingCtx, pingCancel := context.WithTimeout(ctx, testutil.WaitShort)
		defer pingCancel()
		_, err = db.Ping(pingCtx)
		require.NoError(t, err, "ping db")

		// Insert a few orgs.
		org1Name, org1ID := "org1", uuid.New()
		org2Name, org2ID := "org2", uuid.New()
		_, err = db.InsertOrganization(ctx, database.InsertOrganizationParams{
			ID:        org1ID,
			Name:      org1Name,
			CreatedAt: database.Now(),
			UpdatedAt: database.Now(),
		})
		require.NoError(t, err)
		_, err = db.InsertOrganization(ctx, database.InsertOrganizationParams{
			ID:        org2ID,
			Name:      org2Name,
			CreatedAt: database.Now(),
			UpdatedAt: database.Now(),
		})
		require.NoError(t, err)

		root, _ := clitest.New(t,
			"server", "create-admin-user",
			"--postgres-url", connectionURL,
			"--ssh-keygen-algorithm", "ed25519",
			"--username", username,
			"--email", email,
			"--password", password,
		)
		pty := ptytest.New(t)
		root.SetOutput(pty.Output())
		root.SetErr(pty.Output())
		errC := make(chan error, 1)
		go func() {
			err := root.ExecuteContext(ctx)
			t.Log("root.ExecuteContext() returned:", err)
			errC <- err
		}()

		pty.ExpectMatchContext(ctx, "Creating user...")
		pty.ExpectMatchContext(ctx, "Generating user SSH key...")
		pty.ExpectMatchContext(ctx, fmt.Sprintf("Adding user to organization %q (%s) as admin...", org1Name, org1ID.String()))
		pty.ExpectMatchContext(ctx, fmt.Sprintf("Adding user to organization %q (%s) as admin...", org2Name, org2ID.String()))
		pty.ExpectMatchContext(ctx, "User created successfully.")
		pty.ExpectMatchContext(ctx, username)
		pty.ExpectMatchContext(ctx, email)
		pty.ExpectMatchContext(ctx, "****")

		require.NoError(t, <-errC)

		verifyUser(t, connectionURL, username, email, password)
	})

	//nolint:paralleltest
	t.Run("Env", func(t *testing.T) {
		if runtime.GOOS != "linux" || testing.Short() {
			// Skip on non-Linux because it spawns a PostgreSQL instance.
			t.SkipNow()
		}
		connectionURL, closeFunc, err := postgres.Open()
		require.NoError(t, err)
		defer closeFunc()

		// Sometimes generating SSH keys takes a really long time if there isn't
		// enough entropy. We don't want the tests to fail in these cases.
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
		defer cancel()

		t.Setenv("CODER_POSTGRES_URL", connectionURL)
		t.Setenv("CODER_SSH_KEYGEN_ALGORITHM", "ed25519")
		t.Setenv("CODER_USERNAME", username)
		t.Setenv("CODER_EMAIL", email)
		t.Setenv("CODER_PASSWORD", password)

		root, _ := clitest.New(t, "server", "create-admin-user")
		pty := ptytest.New(t)
		root.SetOutput(pty.Output())
		root.SetErr(pty.Output())
		errC := make(chan error, 1)
		go func() {
			err := root.ExecuteContext(ctx)
			t.Log("root.ExecuteContext() returned:", err)
			errC <- err
		}()

		pty.ExpectMatchContext(ctx, "User created successfully.")
		pty.ExpectMatchContext(ctx, username)
		pty.ExpectMatchContext(ctx, email)
		pty.ExpectMatchContext(ctx, "****")

		require.NoError(t, <-errC)

		verifyUser(t, connectionURL, username, email, password)
	})

	t.Run("Stdin", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS != "linux" || testing.Short() {
			// Skip on non-Linux because it spawns a PostgreSQL instance.
			t.SkipNow()
		}
		connectionURL, closeFunc, err := postgres.Open()
		require.NoError(t, err)
		defer closeFunc()

		// Sometimes generating SSH keys takes a really long time if there isn't
		// enough entropy. We don't want the tests to fail in these cases.
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
		defer cancel()

		root, _ := clitest.New(t,
			"server", "create-admin-user",
			"--postgres-url", connectionURL,
			"--ssh-keygen-algorithm", "ed25519",
		)
		pty := ptytest.New(t)
		root.SetIn(pty.Input())
		root.SetOutput(pty.Output())
		root.SetErr(pty.Output())
		errC := make(chan error, 1)
		go func() {
			err := root.ExecuteContext(ctx)
			t.Log("root.ExecuteContext() returned:", err)
			errC <- err
		}()

		pty.ExpectMatchContext(ctx, "> Username")
		pty.WriteLine(username)
		pty.ExpectMatchContext(ctx, "> Email")
		pty.WriteLine(email)
		pty.ExpectMatchContext(ctx, "> Password")
		pty.WriteLine(password)
		pty.ExpectMatchContext(ctx, "> Confirm password")
		pty.WriteLine(password)

		pty.ExpectMatchContext(ctx, "User created successfully.")
		pty.ExpectMatchContext(ctx, username)
		pty.ExpectMatchContext(ctx, email)
		pty.ExpectMatchContext(ctx, "****")

		require.NoError(t, <-errC)

		verifyUser(t, connectionURL, username, email, password)
	})

	t.Run("Validates", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS != "linux" || testing.Short() {
			// Skip on non-Linux because it spawns a PostgreSQL instance.
			t.SkipNow()
		}
		connectionURL, closeFunc, err := postgres.Open()
		require.NoError(t, err)
		defer closeFunc()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		root, _ := clitest.New(t,
			"server", "create-admin-user",
			"--postgres-url", connectionURL,
			"--ssh-keygen-algorithm", "rsa4096",
			"--username", "$",
			"--email", "not-an-email",
			"--password", "x",
		)
		pty := ptytest.New(t)
		root.SetOutput(pty.Output())
		root.SetErr(pty.Output())

		err = root.ExecuteContext(ctx)
		require.Error(t, err)
		require.ErrorContains(t, err, "'email' failed on the 'email' tag")
		require.ErrorContains(t, err, "'username' failed on the 'username' tag")
	})
}
