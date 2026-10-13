package chatstate_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

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

func TestCreateChatInChatProject(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedFamilyDeps(t, db)
	project := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: org.ID, OwnerID: user.ID})

	create := func(projectID, rootID uuid.NullUUID) (database.Chat, error) {
		result, err := chatstate.CreateChat(ctx, db, newRecordingPubsub(), chatstate.CreateChatInput{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			ProjectID:         projectID,
			LastModelConfigID: model.ID,
			Title:             "chat",
			ClientType:        database.ChatClientTypeApi,
			ParentChatID:      rootID,
			RootChatID:        rootID,
			InitialStatus:     database.ChatStatusRunning,
			InitialMessages: []chatstate.Message{
				userTextMessage("hello", user.ID, model.ID),
			},
		})
		return result.Chat, err
	}
	inProject := uuid.NullUUID{UUID: project.ID, Valid: true}
	_, err := create(inProject, uuid.NullUUID{})
	require.NoError(t, err)
	root := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		ProjectID:         inProject,
		Status:            database.ChatStatusWaiting,
	})
	child, err := create(uuid.NullUUID{}, uuid.NullUUID{UUID: root.ID, Valid: true})
	require.NoError(t, err, "sub-chats of a live project's chat are allowed")
	require.Equal(t, inProject, child.ProjectID, "a sub-chat takes its root's project")

	_, err = db.UpdateChatProjectDeletedByID(ctx, project.ID)
	require.NoError(t, err)
	_, err = create(inProject, uuid.NullUUID{})
	require.ErrorIs(t, err, chatstate.ErrChatProjectNotFound)
}

func TestSetFamilyArchivedKeepsDeletedProjectArchived(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedFamilyDeps(t, db)
	project := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: org.ID, OwnerID: user.ID})
	newRoot := func() database.Chat {
		return dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: model.ID,
			ProjectID:         uuid.NullUUID{UUID: project.ID, Valid: true},
			Status:            database.ChatStatusWaiting,
		})
	}
	setArchived := func(rootID uuid.UUID, archived bool) error {
		_, err := chatstate.SetFamilyArchived(ctx, db, newRecordingPubsub(), chatstate.SetFamilyArchivedInput{RootID: rootID, Archived: archived})
		return err
	}

	live := newRoot()
	require.NoError(t, setArchived(live.ID, true))
	require.NoError(t, setArchived(live.ID, false), "a live project's chats can be unarchived")
	chat, err := db.GetChatByID(ctx, live.ID)
	require.NoError(t, err)
	require.False(t, chat.Archived)

	root := newRoot()
	_, err = db.UpdateChatProjectDeletedByID(ctx, project.ID)
	require.NoError(t, err)
	_, err = chatstate.ArchiveDeletedChatProject(ctx, db, newRecordingPubsub(), project.ID)
	require.NoError(t, err)

	_, err = chatstate.SetFamilyArchived(ctx, db, newRecordingPubsub(), chatstate.SetFamilyArchivedInput{
		RootID:   root.ID,
		Archived: false,
	})
	require.ErrorIs(t, err, chatstate.ErrChatNotFound)
	chat, err = db.GetChatByID(ctx, root.ID)
	require.NoError(t, err)
	require.True(t, chat.Archived)
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

// TestArchiveDeletedChatProjectWaitsForChildCreation verifies that the
// cleanup waits for a sub-chat insert holding its root and archives the
// child, and that root chat creation fails once the project is deleted.
func TestArchiveDeletedChatProjectWaitsForChildCreation(t *testing.T) {
	t.Parallel()
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedFamilyDeps(t, db)
	project := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: org.ID, OwnerID: user.ID})
	root := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		ProjectID:         uuid.NullUUID{UUID: project.ID, Valid: true},
	})

	childTx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = childTx.Rollback() }()
	_, err = childTx.ExecContext(ctx, "SELECT id FROM chats WHERE id = $1 FOR SHARE", root.ID)
	require.NoError(t, err)
	childID := uuid.New()
	_, err = childTx.ExecContext(ctx, `INSERT INTO chats (id, organization_id, owner_id, last_model_config_id, title, parent_chat_id, root_chat_id)
		VALUES ($1, $2, $3, $4, 'child', $5, $5)`, childID, org.ID, user.ID, model.ID, root.ID)
	require.NoError(t, err)

	_, err = db.UpdateChatProjectDeletedByID(ctx, project.ID)
	require.NoError(t, err)
	_, err = chatstate.CreateChat(ctx, db, newRecordingPubsub(), chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		ProjectID:         uuid.NullUUID{UUID: project.ID, Valid: true},
		LastModelConfigID: model.ID,
		Title:             "root",
		ClientType:        database.ChatClientTypeApi,
		InitialStatus:     database.ChatStatusWaiting,
		InitialMessages:   []chatstate.Message{userTextMessage("hello", user.ID, model.ID)},
	})
	require.ErrorIs(t, err, chatstate.ErrChatProjectNotFound)

	type cleanupResult struct {
		chats []database.Chat
		err   error
	}
	cleanupDone := make(chan cleanupResult, 1)
	go func() {
		chats, err := chatstate.ArchiveDeletedChatProject(ctx, db, newRecordingPubsub(), project.ID)
		cleanupDone <- cleanupResult{chats: chats, err: err}
	}()
	waitForLockWait(ctx, t, sqlDB, "LockChatProjectRootChats", 1)

	require.NoError(t, childTx.Commit())
	result := testutil.TryReceive(ctx, t, cleanupDone)
	require.NoError(t, result.err)
	ids := make([]uuid.UUID, 0, len(result.chats))
	for _, chat := range result.chats {
		require.True(t, chat.Archived)
		ids = append(ids, chat.ID)
	}
	require.ElementsMatch(t, []uuid.UUID{root.ID, childID}, ids, "a child committed while the cleanup waited is archived")

	again, err := chatstate.ArchiveDeletedChatProject(ctx, db, newRecordingPubsub(), project.ID)
	require.NoError(t, err)
	require.Empty(t, again, "a second cleanup archives nothing")
}

func TestArchiveDeletedChatProjectWaitsForRootCreation(t *testing.T) {
	t.Parallel()
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedFamilyDeps(t, db)
	project := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: org.ID, OwnerID: user.ID})

	created := make(chan uuid.UUID, 1)
	release := make(chan struct{})
	createErr := make(chan error, 1)
	go func() {
		createErr <- db.InTx(func(tx database.Store) error {
			result, err := chatstate.CreateChat(ctx, tx, newRecordingPubsub(), chatstate.CreateChatInput{
				OrganizationID:    org.ID,
				OwnerID:           user.ID,
				ProjectID:         uuid.NullUUID{UUID: project.ID, Valid: true},
				LastModelConfigID: model.ID,
				Title:             "root",
				ClientType:        database.ChatClientTypeApi,
				InitialStatus:     database.ChatStatusWaiting,
				InitialMessages:   []chatstate.Message{userTextMessage("hello", user.ID, model.ID)},
			})
			if err != nil {
				return err
			}
			created <- result.Chat.ID
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}, nil)
	}()
	chatID := testutil.TryReceive(ctx, t, created)

	deleteErr := make(chan error, 1)
	go func() {
		_, err := db.UpdateChatProjectDeletedByID(ctx, project.ID)
		deleteErr <- err
	}()
	waitForLockWait(ctx, t, sqlDB, "UpdateChatProjectDeletedByID", 1)
	close(release)
	require.NoError(t, testutil.TryReceive(ctx, t, createErr))
	require.NoError(t, testutil.TryReceive(ctx, t, deleteErr))

	archived, err := chatstate.ArchiveDeletedChatProject(ctx, db, newRecordingPubsub(), project.ID)
	require.NoError(t, err)
	require.Len(t, archived, 1)
	require.Equal(t, chatID, archived[0].ID, "a root created while the delete waited is archived")
}

func TestArchiveDeletedChatProjectSkipsLiveProject(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedFamilyDeps(t, db)
	project := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: org.ID, OwnerID: user.ID})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		ProjectID:         uuid.NullUUID{UUID: project.ID, Valid: true},
	})

	archived, err := chatstate.ArchiveDeletedChatProject(ctx, db, newRecordingPubsub(), project.ID)
	require.NoError(t, err)
	require.Empty(t, archived)
	chat, err = db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.False(t, chat.Archived)
}

type failingPublisher struct{}

func (failingPublisher) Publish(string, []byte) error {
	return xerrors.New("publish disabled")
}

func TestArchiveDeletedChatProjectReturnsCommittedChatsWhenPublishFails(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedFamilyDeps(t, db)
	project := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: org.ID, OwnerID: user.ID})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		ProjectID:         uuid.NullUUID{UUID: project.ID, Valid: true},
	})
	_, err := db.UpdateChatProjectDeletedByID(ctx, project.ID)
	require.NoError(t, err)

	archived, err := chatstate.ArchiveDeletedChatProject(ctx, db, failingPublisher{}, project.ID)
	require.ErrorIs(t, err, chatstate.ErrChatUpdatePublishFailed)
	require.Len(t, archived, 1)
	require.Equal(t, chat.ID, archived[0].ID)
	require.True(t, archived[0].Archived)
}

// waitForLockWait waits until n sessions are blocked on a row lock while
// running the named query.
func waitForLockWait(ctx context.Context, t *testing.T, sqlDB *sql.DB, queryName string, n int) {
	t.Helper()
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		var waiting int
		err := sqlDB.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM pg_stat_activity
WHERE datname = current_database()
	AND wait_event_type = 'Lock'
	AND query LIKE '%-- name: '||$1||' %'
`, queryName).Scan(&waiting)
		return err == nil && waiting == n
	}, testutil.IntervalFast, "wait for "+queryName+" to block")
	require.NoError(t, ctx.Err())
}
