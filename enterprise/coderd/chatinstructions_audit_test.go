package coderd_test

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	entaudit "github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/testutil"
)

// captureBackend is an enterprise audit backend that records exported logs
// so tests can assert on the production differ's output.
type captureBackend struct {
	mu   sync.Mutex
	logs []database.AuditLog
}

func (*captureBackend) Decision() entaudit.FilterDecision {
	return entaudit.FilterDecisionStore | entaudit.FilterDecisionExport
}

func (b *captureBackend) Export(_ context.Context, alog database.AuditLog, _ entaudit.BackendDetails) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.logs = append(b.logs, alog)
	return nil
}

func (b *captureBackend) reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.logs = nil
}

func (b *captureBackend) entries() []database.AuditLog {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]database.AuditLog, len(b.logs))
	copy(out, b.logs)
	return out
}

// failNextGetChatSystemPromptConfigStore fails the Old capture read of the
// chat system prompt configuration once. Failure state is shared across InTx
// wrappers.
type failNextGetChatSystemPromptConfigStore struct {
	database.Store

	fail *atomic.Bool
}

func newFailNextGetChatSystemPromptConfigStore(store database.Store) *failNextGetChatSystemPromptConfigStore {
	return &failNextGetChatSystemPromptConfigStore{
		Store: store,
		fail:  &atomic.Bool{},
	}
}

func (s *failNextGetChatSystemPromptConfigStore) InTx(function func(database.Store) error, txOpts *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error {
		return function(&failNextGetChatSystemPromptConfigStore{
			Store: tx,
			fail:  s.fail,
		})
	}, txOpts)
}

func (s *failNextGetChatSystemPromptConfigStore) GetChatSystemPromptConfig(ctx context.Context) (database.GetChatSystemPromptConfigRow, error) {
	if s.fail.CompareAndSwap(true, false) {
		return database.GetChatSystemPromptConfigRow{}, stderrors.New("forced old capture failure")
	}
	return s.Store.GetChatSystemPromptConfig(ctx)
}

// TestChatInstructionSettingsOldCaptureFailsClosed proves a failed baseline
// read rejects the mutation and records the failed attempt without fabricating
// a diff from a zero value.
func TestChatInstructionSettingsOldCaptureFailsClosed(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)

	rawDB, pubsub := dbtestutil.NewDB(t)
	store := newFailNextGetChatSystemPromptConfigStore(rawDB)
	backend := &captureBackend{}
	auditor := entaudit.NewAuditor(rawDB, entaudit.DefaultFilter, backend)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	rawClient, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{
		Database:         store,
		Pubsub:           pubsub,
		DeploymentValues: coderdtest.DeploymentValues(t),
		Auditor:          auditor,
		Logger:           &logger,
	})
	client := codersdk.NewExperimentalClient(rawClient)
	_ = coderdtest.CreateFirstUser(t, client.Client)
	// Discard the login entry emitted by user creation.
	backend.reset()

	store.fail.Store(true)
	err := client.UpdateChatSystemPrompt(ctx, codersdk.UpdateChatSystemPromptRequest{
		SystemPrompt:               "Must not be stored.",
		IncludeDefaultSystemPrompt: ptr.Ref(true),
	})
	sdkErr := new(codersdk.Error)
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusInternalServerError, sdkErr.StatusCode())

	entries := backend.entries()
	require.Len(t, entries, 1)
	require.EqualValues(t, http.StatusInternalServerError, entries[0].StatusCode)
	require.JSONEq(t, "{}", string(entries[0].Diff))

	resp, err := client.GetChatSystemPrompt(ctx)
	require.NoError(t, err)
	require.Empty(t, resp.SystemPrompt)
	require.True(t, resp.IncludeDefaultSystemPrompt)
}

// TestChatInstructionSettingsIncludeDefaultPresence exercises the production
// differ against the include-default presence transition: writing an explicit
// false over a legacy absent override row does not move the effective value
// but inserts the persistent row, which changes future behavior, so the
// presence transition must be audited.
func TestChatInstructionSettingsIncludeDefaultPresence(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)

	rawDB, pubsub := dbtestutil.NewDB(t)
	backend := &captureBackend{}
	auditor := entaudit.NewAuditor(rawDB, entaudit.DefaultFilter, backend)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	rawClient, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{
		Database:         rawDB,
		Pubsub:           pubsub,
		DeploymentValues: coderdtest.DeploymentValues(t),
		Auditor:          auditor,
		Logger:           &logger,
	})
	client := codersdk.NewExperimentalClient(rawClient)
	_ = coderdtest.CreateFirstUser(t, client.Client)
	// Discard the login entry emitted by user creation.
	backend.reset()

	// Legacy shape: a non-empty prompt with no override row computes
	// effective false.
	err := client.UpdateChatSystemPrompt(ctx, codersdk.UpdateChatSystemPromptRequest{
		SystemPrompt: "Legacy custom instructions.",
	})
	require.NoError(t, err)
	require.Len(t, backend.entries(), 1)

	// Explicit false over the absent row: same effective value, but the
	// override row now exists, so the entry must record the presence
	// transition.
	backend.reset()
	err = client.UpdateChatSystemPrompt(ctx, codersdk.UpdateChatSystemPromptRequest{
		SystemPrompt:               "Legacy custom instructions.",
		IncludeDefaultSystemPrompt: ptr.Ref(false),
	})
	require.NoError(t, err)
	require.Len(t, backend.entries(), 1)
	var diff map[string]codersdk.AuditDiffField
	require.NoError(t, json.Unmarshal(backend.entries()[0].Diff, &diff))
	require.Equal(t, codersdk.AuditDiffField{Old: false, New: true},
		diff["include_default_system_prompt_set"])
	// The effective value did not move, and the prompt is unchanged.
	require.NotContains(t, diff, "include_default_system_prompt")
	require.NotContains(t, diff, "system_prompt")

	// Repeating the same explicit-false PUT is a true no-op now: the row
	// exists with the same value, so nothing is audited.
	backend.reset()
	err = client.UpdateChatSystemPrompt(ctx, codersdk.UpdateChatSystemPromptRequest{
		SystemPrompt:               "Legacy custom instructions.",
		IncludeDefaultSystemPrompt: ptr.Ref(false),
	})
	require.NoError(t, err)
	require.Empty(t, backend.entries())
}
