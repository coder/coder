package chatd_test

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// textPartWithMetadata returns a text part carrying dedup metadata,
// the shape slackd submits.
func textPartWithMetadata(text, key, value string) []codersdk.ChatMessagePart {
	return []codersdk.ChatMessagePart{{
		Type:     codersdk.ChatMessagePartTypeText,
		Text:     text,
		Metadata: map[string]string{key: value},
	}}
}

func TestSendMessageDedupMetadata(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newTestServer(t, db, ps, uuid.New())
	user, org, model := seedChatDependencies(t, db)
	apiKeyID := testAPIKeyID(t, db, user.ID)

	const key = "slack_event_id"

	t.Run("HistoryAndQueue", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		// The initial user message carries event id "ev-1" and lands
		// in history; the chat stays running (no worker), so
		// follow-up sends land in the queue.
		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID:     org.ID,
			OwnerID:            user.ID,
			APIKeyID:           apiKeyID,
			Title:              "dedup test",
			InitialUserContent: textPartWithMetadata("first", key, "ev-1"),
			ModelConfigID:      model.ID,
		})
		require.NoError(t, err)

		// Duplicate of the history message is rejected.
		_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:           chat.ID,
			APIKeyID:         apiKeyID,
			Content:          textPartWithMetadata("first again", key, "ev-1"),
			DedupMetadataKey: key,
		})
		require.ErrorIs(t, err, chatd.ErrDuplicateMessage)

		// A new event id passes and is queued.
		result, err := server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:           chat.ID,
			APIKeyID:         apiKeyID,
			Content:          textPartWithMetadata("second", key, "ev-2"),
			DedupMetadataKey: key,
		})
		require.NoError(t, err)
		require.True(t, result.Queued)

		// Duplicate of the queued message is rejected too.
		_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:           chat.ID,
			APIKeyID:         apiKeyID,
			Content:          textPartWithMetadata("second again", key, "ev-2"),
			DedupMetadataKey: key,
		})
		require.ErrorIs(t, err, chatd.ErrDuplicateMessage)

		// Without a dedup key nothing is rejected.
		_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:   chat.ID,
			APIKeyID: apiKeyID,
			Content:  textPartWithMetadata("second again", key, "ev-2"),
		})
		require.NoError(t, err)
	})

	t.Run("KeyMissingFromContent", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello")
		_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:           chat.ID,
			APIKeyID:         apiKeyID,
			Content:          []codersdk.ChatMessagePart{codersdk.ChatMessageText("no metadata")},
			DedupMetadataKey: key,
		})
		require.Error(t, err)
		require.NotErrorIs(t, err, chatd.ErrDuplicateMessage)
	})

	t.Run("Concurrent", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID:     org.ID,
			OwnerID:            user.ID,
			APIKeyID:           apiKeyID,
			Title:              "dedup concurrent test",
			InitialUserContent: textPartWithMetadata("first", key, "ev-conc-0"),
			ModelConfigID:      model.ID,
		})
		require.NoError(t, err)

		const submitters = 4
		errs := make([]error, submitters)
		var wg sync.WaitGroup
		for i := range submitters {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = server.SendMessage(ctx, chatd.SendMessageOptions{
					ChatID:           chat.ID,
					APIKeyID:         apiKeyID,
					Content:          textPartWithMetadata("same event", key, "ev-conc-1"),
					DedupMetadataKey: key,
				})
			}()
		}
		wg.Wait()

		var accepted, duplicates int
		for _, err := range errs {
			switch {
			case err == nil:
				accepted++
			case xerrors.Is(err, chatd.ErrDuplicateMessage):
				duplicates++
			default:
				t.Fatalf("unexpected error: %v", err)
			}
		}
		require.Equal(t, 1, accepted)
		require.Equal(t, submitters-1, duplicates)
	})
}

func TestCreateChatDedupLabels(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newTestServer(t, db, ps, uuid.New())
	user, org, model := seedChatDependencies(t, db)
	apiKeyID := testAPIKeyID(t, db, user.ID)

	t.Run("SecondCreateReturnsExisting", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		labels := map[string]string{"slackd": "true", "slack_thread": "C1:100.1"}
		opts := chatd.CreateOptions{
			OrganizationID:     org.ID,
			OwnerID:            user.ID,
			APIKeyID:           apiKeyID,
			Title:              "slack thread chat",
			InitialUserContent: textPartWithMetadata("first", "slack_event_id", "ev-1"),
			ModelConfigID:      model.ID,
			Labels:             database.StringMap(labels),
			DedupLabels:        labels,
		}
		created, err := server.CreateChat(ctx, opts)
		require.NoError(t, err)

		existing, err := server.CreateChat(ctx, opts)
		require.ErrorIs(t, err, chatd.ErrChatAlreadyExists)
		require.Equal(t, created.ID, existing.ID)
	})

	t.Run("DedupLabelsMustBeSubsetOfLabels", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID:     org.ID,
			OwnerID:            user.ID,
			APIKeyID:           apiKeyID,
			Title:              "bad dedup labels",
			InitialUserContent: textPartWithMetadata("first", "slack_event_id", "ev-2"),
			ModelConfigID:      model.ID,
			Labels:             database.StringMap{"a": "1"},
			DedupLabels:        map[string]string{"b": "2"},
		})
		require.Error(t, err)
	})

	t.Run("Concurrent", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		labels := map[string]string{"slackd": "true", "slack_thread": "C1:200.2"}
		const creators = 4
		chats := make([]database.Chat, creators)
		errs := make([]error, creators)
		var wg sync.WaitGroup
		for i := range creators {
			wg.Add(1)
			go func() {
				defer wg.Done()
				chats[i], errs[i] = server.CreateChat(ctx, chatd.CreateOptions{
					OrganizationID:     org.ID,
					OwnerID:            user.ID,
					APIKeyID:           apiKeyID,
					Title:              "slack thread chat",
					InitialUserContent: textPartWithMetadata("first", "slack_event_id", "ev-3"),
					ModelConfigID:      model.ID,
					Labels:             database.StringMap(labels),
					DedupLabels:        labels,
				})
			}()
		}
		wg.Wait()

		var created, existing int
		var chatID uuid.UUID
		for i, err := range errs {
			switch {
			case err == nil:
				created++
				chatID = chats[i].ID
			case xerrors.Is(err, chatd.ErrChatAlreadyExists):
				existing++
			default:
				t.Fatalf("unexpected error: %v", err)
			}
		}
		require.Equal(t, 1, created)
		require.Equal(t, creators-1, existing)
		// Every loser was handed the winner's chat.
		for i, err := range errs {
			if xerrors.Is(err, chatd.ErrChatAlreadyExists) {
				require.Equal(t, chatID, chats[i].ID)
			}
		}
	})
}
