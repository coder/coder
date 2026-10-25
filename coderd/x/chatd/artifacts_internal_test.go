package chatd

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/testutil"
)

type artifactTestEnv struct {
	f *workerTestFixture
	// authzDB is the store that chatd uses: dbauthz over the test DB.
	authzDB database.Store
}

func newArtifactTestEnv(t *testing.T) *artifactTestEnv {
	t.Helper()
	f := newWorkerTestFixture(t)
	authzDB := dbauthz.New(f.db, rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()), slogtest.Make(t, nil), nil)
	return &artifactTestEnv{f: f, authzDB: authzDB}
}

// member creates a user in the fixture organization.
func (e *artifactTestEnv) member(t *testing.T) database.User {
	t.Helper()
	user := dbgen.User(t, e.f.db, database.User{})
	dbgen.OrganizationMember(t, e.f.db, database.OrganizationMember{UserID: user.ID, OrganizationID: e.f.org.ID})
	return user
}

func (e *artifactTestEnv) chat(t *testing.T, seed database.Chat) database.Chat {
	t.Helper()
	if seed.OrganizationID == uuid.Nil {
		seed.OrganizationID = e.f.org.ID
	}
	if seed.OwnerID == uuid.Nil {
		seed.OwnerID = e.f.user.ID
	}
	seed.LastModelConfigID = e.f.model.ID
	return dbgen.Chat(t, e.f.db, seed)
}

func (e *artifactTestEnv) store(chat database.Chat) *artifactStore {
	return newArtifactStore(e.authzDB, chat)
}

// sharedProject creates a project owned by the fixture user and shared
// with reader through the project ACL.
func (e *artifactTestEnv) sharedProject(t *testing.T, reader database.User) database.ChatProject {
	t.Helper()
	project := dbgen.ChatProject(t, e.f.db, database.ChatProject{OrganizationID: e.f.org.ID, OwnerID: e.f.user.ID})
	err := e.f.db.UpdateChatProjectACLByID(testutil.Context(t, testutil.WaitShort), database.UpdateChatProjectACLByIDParams{
		ID:       project.ID,
		UserACL:  database.ChatACL{reader.ID.String(): {Permissions: []policy.Action{policy.ActionRead}}},
		GroupACL: database.ChatACL{},
	})
	require.NoError(t, err)
	return project
}

func publishParams(base int32, id uuid.UUID, data string) chattool.PublishArtifactParams {
	return chattool.PublishArtifactParams{
		ArtifactID:  uuid.NullUUID{UUID: id, Valid: id != uuid.Nil},
		BaseVersion: base,
		Title:       "Report",
		Name:        "report.md",
		Data:        []byte(data),
	}
}

func requireStale(t *testing.T, err error, latest int32) {
	t.Helper()
	var stale *chattool.StaleBaseError
	require.ErrorAs(t, err, &stale)
	require.Equal(t, latest, stale.Latest)
}

func TestArtifactStore(t *testing.T) {
	t.Parallel()

	t.Run("NewArtifact", func(t *testing.T) {
		t.Parallel()
		e := newArtifactTestEnv(t)
		ctx := testutil.Context(t, testutil.WaitMedium)
		chat := e.chat(t, database.Chat{})

		v1, err := e.store(chat).Publish(ctx, publishParams(0, uuid.Nil, "# v1"))
		require.NoError(t, err)
		require.Equal(t, int32(1), v1.Version)
		require.Equal(t, "Report", v1.Title)
		require.Equal(t, "report.md", v1.Name)

		row, err := e.f.db.GetChatArtifactByID(ctx, v1.ArtifactID)
		require.NoError(t, err)
		require.Equal(t, chat.OrganizationID, row.ChatArtifact.OrganizationID)
		require.Equal(t, chat.OwnerID, row.ChatArtifact.OwnerID)
		require.False(t, row.ChatArtifact.ProjectID.Valid)
		require.Equal(t, int32(1), row.LatestVersion)

		version, err := e.f.db.GetChatArtifactVersion(ctx, database.GetChatArtifactVersionParams{ArtifactID: v1.ArtifactID, Version: 1})
		require.NoError(t, err)
		require.Equal(t, "text/markdown", version.Mimetype)
		require.Equal(t, uuid.NullUUID{UUID: chat.ID, Valid: true}, version.ChatID)
		require.Equal(t, uuid.NullUUID{UUID: chat.OwnerID, Valid: true}, version.CreatedBy)
	})

	t.Run("NewArtifactTakesUsableChatProject", func(t *testing.T) {
		t.Parallel()
		e := newArtifactTestEnv(t)
		ctx := testutil.Context(t, testutil.WaitMedium)
		reader := e.member(t)
		project := e.sharedProject(t, reader)
		chat := e.chat(t, database.Chat{OwnerID: reader.ID, ProjectID: uuid.NullUUID{UUID: project.ID, Valid: true}})

		v1, err := e.store(chat).Publish(ctx, publishParams(0, uuid.Nil, "# v1"))
		require.NoError(t, err)
		row, err := e.f.db.GetChatArtifactByID(ctx, v1.ArtifactID)
		require.NoError(t, err)
		require.Equal(t, uuid.NullUUID{UUID: project.ID, Valid: true}, row.ChatArtifact.ProjectID)
		require.Equal(t, project.Name, row.ProjectName)
	})

	t.Run("NextVersion", func(t *testing.T) {
		t.Parallel()
		e := newArtifactTestEnv(t)
		ctx := testutil.Context(t, testutil.WaitMedium)
		chat := e.chat(t, database.Chat{})
		store := e.store(chat)

		v1, err := store.Publish(ctx, publishParams(0, uuid.Nil, "# v1"))
		require.NoError(t, err)
		params := publishParams(1, v1.ArtifactID, "# v2")
		params.Title = ""
		params.Label = "second"
		v2, err := store.Publish(ctx, params)
		require.NoError(t, err)
		require.Equal(t, v1.ArtifactID, v2.ArtifactID)
		require.Equal(t, int32(2), v2.Version)
		require.Equal(t, "Report", v2.Title, "an empty title keeps the title")

		got, err := store.Read(ctx, v1.ArtifactID, 0)
		require.NoError(t, err)
		require.Equal(t, int32(2), got.Version)
		require.Equal(t, []byte("# v2"), got.Data)
		got, err = store.Read(ctx, v1.ArtifactID, 1)
		require.NoError(t, err)
		require.Equal(t, []byte("# v1"), got.Data, "old versions keep their bytes")
	})

	t.Run("OldBase", func(t *testing.T) {
		t.Parallel()
		e := newArtifactTestEnv(t)
		ctx := testutil.Context(t, testutil.WaitMedium)
		store := e.store(e.chat(t, database.Chat{}))

		v1, err := store.Publish(ctx, publishParams(0, uuid.Nil, "# v1"))
		require.NoError(t, err)
		_, err = store.Publish(ctx, publishParams(1, v1.ArtifactID, "# v2"))
		require.NoError(t, err)
		_, err = store.Publish(ctx, publishParams(1, v1.ArtifactID, "# v2 again"))
		requireStale(t, err, 2)
		_, err = store.Publish(ctx, publishParams(5, v1.ArtifactID, "# future"))
		requireStale(t, err, 2)
	})

	t.Run("TwoPublishesOnOneBase", func(t *testing.T) {
		t.Parallel()
		e := newArtifactTestEnv(t)
		ctx := testutil.Context(t, testutil.WaitMedium)
		chatA := e.chat(t, database.Chat{})
		chatB := e.chat(t, database.Chat{})
		v1, err := e.store(chatA).Publish(ctx, publishParams(0, uuid.Nil, "# v1"))
		require.NoError(t, err)

		// Both chats publish on v1 at the same time. Exactly one wins,
		// and the other learns that v2 is the latest.
		var wg sync.WaitGroup
		errs := make([]error, 2)
		for i, chat := range []database.Chat{chatA, chatB} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = e.store(chat).Publish(ctx, publishParams(1, v1.ArtifactID, "# v2"))
			}()
		}
		wg.Wait()
		var won int
		for _, err := range errs {
			if err == nil {
				won++
				continue
			}
			requireStale(t, err, 2)
		}
		require.Equal(t, 1, won)

		// A sequential second publish on the same base is stale too.
		_, err = e.store(chatB).Publish(ctx, publishParams(1, v1.ArtifactID, "# v2 from B"))
		requireStale(t, err, 2)
	})

	t.Run("OtherUsersPersonalArtifact", func(t *testing.T) {
		t.Parallel()
		e := newArtifactTestEnv(t)
		ctx := testutil.Context(t, testutil.WaitMedium)
		v1, err := e.store(e.chat(t, database.Chat{})).Publish(ctx, publishParams(0, uuid.Nil, "# v1"))
		require.NoError(t, err)

		stranger := e.member(t)
		store := e.store(e.chat(t, database.Chat{OwnerID: stranger.ID}))
		_, err = store.Read(ctx, v1.ArtifactID, 0)
		require.ErrorIs(t, err, chattool.ErrArtifactNotFound)
		_, err = store.Publish(ctx, publishParams(1, v1.ArtifactID, "# v2"))
		require.ErrorIs(t, err, chattool.ErrArtifactNotFound)
	})

	t.Run("OtherOrganization", func(t *testing.T) {
		t.Parallel()
		e := newArtifactTestEnv(t)
		ctx := testutil.Context(t, testutil.WaitMedium)
		v1, err := e.store(e.chat(t, database.Chat{})).Publish(ctx, publishParams(0, uuid.Nil, "# v1"))
		require.NoError(t, err)

		// The same owner in another organization cannot use it.
		other := dbgen.Organization(t, e.f.db, database.Organization{})
		dbgen.OrganizationMember(t, e.f.db, database.OrganizationMember{UserID: e.f.user.ID, OrganizationID: other.ID})
		store := e.store(e.chat(t, database.Chat{OrganizationID: other.ID}))
		_, err = store.Read(ctx, v1.ArtifactID, 0)
		require.ErrorIs(t, err, chattool.ErrArtifactNotFound)
		_, err = store.Publish(ctx, publishParams(1, v1.ArtifactID, "# v2"))
		require.ErrorIs(t, err, chattool.ErrArtifactNotFound)
	})

	t.Run("ProjectMemberAndUntrusted", func(t *testing.T) {
		t.Parallel()
		e := newArtifactTestEnv(t)
		ctx := testutil.Context(t, testutil.WaitMedium)
		reader := e.member(t)
		project := e.sharedProject(t, reader)
		ownerChat := e.chat(t, database.Chat{ProjectID: uuid.NullUUID{UUID: project.ID, Valid: true}})
		ownerStore := e.store(ownerChat)
		v1, err := ownerStore.Publish(ctx, publishParams(0, uuid.Nil, "# v1"))
		require.NoError(t, err)

		// The member reads the owner's version as untrusted, then
		// publishes v2 from a chat outside the project.
		readerStore := e.store(e.chat(t, database.Chat{OwnerID: reader.ID}))
		got, err := readerStore.Read(ctx, v1.ArtifactID, 0)
		require.NoError(t, err)
		require.Equal(t, int32(1), got.Version)
		require.True(t, got.Untrusted)
		_, err = readerStore.Publish(ctx, publishParams(1, v1.ArtifactID, "# v2 from member"))
		require.NoError(t, err)

		// The owner sees the member's v2 as untrusted, and its own v1 not.
		got, err = ownerStore.Read(ctx, v1.ArtifactID, 0)
		require.NoError(t, err)
		require.Equal(t, int32(2), got.Version)
		require.True(t, got.Untrusted)
		got, err = ownerStore.Read(ctx, v1.ArtifactID, 1)
		require.NoError(t, err)
		require.False(t, got.Untrusted)

		// Removing the member from the project ACL removes access.
		err = e.f.db.UpdateChatProjectACLByID(ctx, database.UpdateChatProjectACLByIDParams{ID: project.ID, UserACL: database.ChatACL{}, GroupACL: database.ChatACL{}})
		require.NoError(t, err)
		_, err = readerStore.Read(ctx, v1.ArtifactID, 0)
		require.ErrorIs(t, err, chattool.ErrArtifactNotFound)
	})

	t.Run("DeletedChatKeepsVersions", func(t *testing.T) {
		t.Parallel()
		e := newArtifactTestEnv(t)
		ctx := testutil.Context(t, testutil.WaitMedium)
		chat := e.chat(t, database.Chat{})
		v1, err := e.store(chat).Publish(ctx, publishParams(0, uuid.Nil, "# v1"))
		require.NoError(t, err)

		_, err = e.f.sqlDB.ExecContext(ctx, "DELETE FROM chats WHERE id = $1", chat.ID)
		require.NoError(t, err)

		version, err := e.f.db.GetChatArtifactVersion(ctx, database.GetChatArtifactVersionParams{ArtifactID: v1.ArtifactID, Version: 1})
		require.NoError(t, err)
		require.False(t, version.ChatID.Valid)
		require.Equal(t, []byte("# v1"), version.Data)

		// A later chat continues the artifact.
		got, err := e.store(e.chat(t, database.Chat{})).Read(ctx, v1.ArtifactID, 0)
		require.NoError(t, err)
		require.Equal(t, []byte("# v1"), got.Data)
	})
}

func TestArtifactToolsPlanMode(t *testing.T) {
	t.Parallel()
	for _, name := range []string{chattool.PublishArtifactToolName, chattool.ReadArtifactToolName} {
		require.True(t, builtinPlanToolAllowed(name, true), name)
		require.False(t, builtinPlanToolAllowed(name, false), "%s is a root-chat tool", name)
	}
}
