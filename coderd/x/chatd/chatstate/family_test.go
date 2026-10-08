package chatstate_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestSetFamilyArchivedRejectsChildChat asserts the chatstate helper
// rejects calls that target a child chat. Family archive flows must
// always start at the root.
func TestSetFamilyArchivedRejectsChildChat(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	root := dbgen.Chat(t, f.DB, database.Chat{
		OrganizationID:    f.Org.ID,
		OwnerID:           f.User.ID,
		LastModelConfigID: f.Model.ID,
		Title:             "root",
	})
	child := dbgen.Chat(t, f.DB, database.Chat{
		OrganizationID:    f.Org.ID,
		OwnerID:           f.User.ID,
		LastModelConfigID: f.Model.ID,
		Title:             "child",
		ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
	})

	_, err := chatstate.SetFamilyArchived(ctx, f.DB, f.Pub, chatstate.SetFamilyArchivedInput{RootID: child.ID, Archived: true})
	require.ErrorIs(t, err, chatstate.ErrChatNotRoot)

	require.False(t, f.readChat(ctx, t, root.ID).Archived,
		"failed family archive must not touch the root")
	require.False(t, f.readChat(ctx, t, child.ID).Archived,
		"failed family archive must not touch the child")
}

// TestSetFamilyArchivedRollsBackWhenMemberCannotArchive verifies that
// SetFamilyArchived is atomic: when one family member is in a state
// that cannot satisfy the SetArchived transition, the whole cascade
// rolls back and no publications reach the inner publisher.
func TestSetFamilyArchivedRollsBackWhenMemberCannotArchive(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	user, org, model := seedFamilyDeps(t, db)

	// Root chat: waiting is archive-eligible (state W).
	root := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "root",
		Status:            database.ChatStatusWaiting,
	})
	// Child chat: running with no queue is R0 and NOT archive
	// eligible per the chatstate transition matrix.
	child := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "child",
		Status:            database.ChatStatusRunning,
		ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
	})

	pub := newRecordingPubsub()
	_, err := chatstate.SetFamilyArchived(ctx, db, pub, chatstate.SetFamilyArchivedInput{RootID: root.ID, Archived: true})
	require.Error(t, err, "child in "+chatstate.StateR0.String()+" must reject SetArchived")
	require.ErrorIs(t, err, chatstate.ErrTransitionNotAllowed)

	rootAfter, err := db.GetChatByID(ctx, root.ID)
	require.NoError(t, err)
	require.False(t, rootAfter.Archived, "root archive must roll back when a child cannot archive")
	childAfter, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.False(t, childAfter.Archived, "child must not be archived in the rolled-back cascade")

	require.Empty(t, pub.channels,
		"rolled-back family archive must publish nothing through the inner publisher")
}

// TestSetFamilyArchivedRejectsInvalidStateEvenWhenAlreadyDesired
// verifies that invalid-state detection is never bypassed: a family
// member in StateInvalid causes the cascade to fail with
// ErrInvalidState even when that member's archived flag already
// matches the desired value.
func TestSetFamilyArchivedRejectsInvalidStateEvenWhenAlreadyDesired(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	user, org, model := seedFamilyDeps(t, db)

	root := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "root",
		Status:            database.ChatStatusWaiting,
	})
	child := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "child",
		// status=waiting, archived=true; we will add a queued message
		// to produce the chatstate-invalid combination (archived chat
		// with a queued backlog is outside the valid state model).
		Status:       database.ChatStatusWaiting,
		Archived:     true,
		ParentChatID: uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:   uuid.NullUUID{UUID: root.ID, Valid: true},
	})

	// Seed a queued message under the child to push it into the
	// chatstate-invalid combination.
	rawContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("queued"),
	})
	require.NoError(t, err)
	_, err = db.InsertChatQueuedMessage(ctx, database.InsertChatQueuedMessageParams{
		ChatID:        child.ID,
		Content:       rawContent.RawMessage,
		ModelConfigID: uuid.NullUUID{},
	})
	require.NoError(t, err)

	pub := newRecordingPubsub()
	_, err = chatstate.SetFamilyArchived(ctx, db, pub, chatstate.SetFamilyArchivedInput{
		RootID:   root.ID,
		Archived: true,
	})
	require.ErrorIs(t, err, chatstate.ErrInvalidState,
		"invalid-state child blocks the cascade even when archived flag already matches")

	// Root must not be archived because the cascade rolled back.
	rootAfter, err := db.GetChatByID(ctx, root.ID)
	require.NoError(t, err)
	require.False(t, rootAfter.Archived, "root must roll back when a child is in StateInvalid")

	require.Empty(t, pub.channels,
		"rolled-back cascade must not publish anything")
}

// TestSetFamilyArchivedAcceptsAlreadyDesiredMembers verifies that an
// individually archived child does not block a root archive cascade.
// The cascade converges to the desired state even when some family
// members already match it.
func TestSetFamilyArchivedAcceptsAlreadyDesiredMembers(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	user, org, model := seedFamilyDeps(t, db)

	root := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "root",
		Status:            database.ChatStatusWaiting,
	})
	child := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "child",
		Status:            database.ChatStatusWaiting,
		ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
		Archived:          true,
	})

	pub := newRecordingPubsub()
	family, err := chatstate.SetFamilyArchived(ctx, db, pub, chatstate.SetFamilyArchivedInput{RootID: root.ID, Archived: true})
	require.NoError(t, err,
		"already archived members must not block the cascade")
	require.Len(t, family, 2)

	rootAfter, err := db.GetChatByID(ctx, root.ID)
	require.NoError(t, err)
	require.True(t, rootAfter.Archived)
	childAfter, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.True(t, childAfter.Archived)
}

// TestCreateChildChatWaitsForFamilyArchive verifies that creating a
// child chat serializes with SetFamilyArchived. The child creation
// blocks on the root lock the archive holds, then observes the
// committed archive and refuses to join the archived family.
func TestCreateChildChatWaitsForFamilyArchive(t *testing.T) {
	t.Parallel()
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedFamilyDeps(t, db)

	root := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "root",
		Status:            database.ChatStatusWaiting,
	})

	archived := make(chan struct{})
	release := make(chan struct{})
	archiveErr := make(chan error, 1)
	go func() {
		archiveErr <- db.InTx(func(tx database.Store) error {
			// SetFamilyArchived reuses the outer transaction, so its
			// root lock is held until the test releases it.
			_, err := chatstate.SetFamilyArchived(ctx, tx, newRecordingPubsub(), chatstate.SetFamilyArchivedInput{
				RootID:   root.ID,
				Archived: true,
			})
			if err != nil {
				return err
			}
			close(archived)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}, nil)
	}()
	select {
	case <-archived:
	case err := <-archiveErr:
		t.Fatalf("archive failed before holding the root lock: %v", err)
	case <-ctx.Done():
		t.Fatal("timed out waiting for the archive to lock the root")
	}

	createErr := make(chan error, 1)
	go func() {
		_, err := chatstate.CreateChat(ctx, db, newRecordingPubsub(), chatstate.CreateChatInput{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: model.ID,
			Title:             "child",
			ClientType:        database.ChatClientTypeApi,
			ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
			InitialStatus:     database.ChatStatusRunning,
			InitialMessages: []chatstate.Message{
				userTextMessage("hello", user.ID, model.ID),
			},
		})
		createErr <- err
	}()

	// Wait until the child creation blocks on the root row lock.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		var waiting int
		err := sqlDB.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM pg_stat_activity
WHERE datname = current_database()
	AND pid <> pg_backend_pid()
	AND wait_event_type = 'Lock'
	AND query LIKE '%-- name: GetChatByIDForShare%'
`).Scan(&waiting)
		return err == nil && waiting == 1
	}, testutil.IntervalFast, "wait for child creation to block on the root lock")
	require.NoError(t, ctx.Err(), "waiting for child creation to block")
	select {
	case err := <-createErr:
		t.Fatalf("child creation finished while the archive held the root lock: %v", err)
	default:
	}

	close(release)
	select {
	case err := <-archiveErr:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("timed out waiting for the archive to commit")
	}
	select {
	case err := <-createErr:
		require.ErrorIs(t, err, chatstate.ErrChatFamilyArchived)
	case <-ctx.Done():
		t.Fatal("timed out waiting for child creation")
	}

	ids, err := db.GetChatFamilyIDsByRootID(ctx, root.ID)
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{root.ID}, ids, "no child may join the archived family")
}

func TestCreateChildChatRejectsDeletedProject(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedFamilyDeps(t, db)

	project := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: org.ID, OwnerID: user.ID})
	root := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		ProjectID:         uuid.NullUUID{UUID: project.ID, Valid: true},
		Status:            database.ChatStatusWaiting,
	})
	require.NoError(t, db.MarkChatProjectDeleted(ctx, project.ID))

	_, err := chatstate.CreateChat(ctx, db, newRecordingPubsub(), chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "child",
		ClientType:        database.ChatClientTypeApi,
		ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
		InitialStatus:     database.ChatStatusRunning,
		InitialMessages: []chatstate.Message{
			userTextMessage("hello", user.ID, model.ID),
		},
	})
	require.ErrorIs(t, err, chatstate.ErrChatNotFound)
}

func seedFamilyDeps(t *testing.T, db database.Store) (database.User, database.Organization, database.ChatModelConfig) {
	t.Helper()
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    "openai",
		DisplayName: "openai",
		BaseUrl:     "http://example.invalid",
	})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		IsDefault: true,
	})
	return user, org, model
}
