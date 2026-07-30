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

// failNextChatSystemPromptConfigStore fails the New re-read of the chat
// system prompt configuration once: the first read inside the audited
// transaction (the Old capture) and every later read succeed, the second
// fails. Failure state is shared across InTx wrappers.
type failNextChatSystemPromptConfigStore struct {
	database.Store

	calls *atomic.Int64
	fail  *atomic.Bool
}

func newFailNextChatSystemPromptConfigStore(store database.Store) *failNextChatSystemPromptConfigStore {
	return &failNextChatSystemPromptConfigStore{
		Store: store,
		calls: &atomic.Int64{},
		fail:  &atomic.Bool{},
	}
}

func (s *failNextChatSystemPromptConfigStore) InTx(function func(database.Store) error, txOpts *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error {
		s.calls.Store(0)
		return function(&failNextChatSystemPromptConfigStore{
			Store: tx,
			calls: s.calls,
			fail:  s.fail,
		})
	}, txOpts)
}

func (s *failNextChatSystemPromptConfigStore) GetChatSystemPromptConfig(ctx context.Context) (database.GetChatSystemPromptConfigRow, error) {
	// The second read inside the audited transaction is the New re-read.
	if s.calls.Add(1) == 2 && s.fail.CompareAndSwap(true, false) {
		return database.GetChatSystemPromptConfigRow{}, stderrors.New("forced new capture failure")
	}
	return s.Store.GetChatSystemPromptConfig(ctx)
}

// TestChatInstructionSettingsDegradedNewCapture exercises the production
// enterprise differ against the chat instruction settings handlers, which the
// empty-diff mock auditor cannot do: a degraded New capture must yield an
// unknown value, never a zero value, so the diff is empty rather than a
// fabricated deletion.
func TestChatInstructionSettingsDegradedNewCapture(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)

	rawDB, pubsub := dbtestutil.NewDB(t)
	store := newFailNextChatSystemPromptConfigStore(rawDB)
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

	err := client.UpdateChatSystemPrompt(ctx, codersdk.UpdateChatSystemPromptRequest{
		SystemPrompt:               "Baseline prompt.",
		IncludeDefaultSystemPrompt: ptr.Ref(true),
	})
	require.NoError(t, err)
	require.Len(t, backend.entries(), 1)

	// Fail the New re-read only: the Old capture succeeds, the write
	// succeeds, and the re-read fails. Because the read is fatal on
	// measured unreachability (a statement error aborts the transaction),
	// the request fails and rolls back, so no row can carry a fabricated
	// Old-to-empty deletion.
	store.fail.Store(true)
	err = client.UpdateChatSystemPrompt(ctx, codersdk.UpdateChatSystemPromptRequest{
		SystemPrompt: "Changed prompt.",
	})
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusInternalServerError, sdkErr.StatusCode())
	// The failed request records the attempt with an empty diff; no
	// fabricated deletion can appear.
	require.Len(t, backend.entries(), 2)
	require.JSONEq(t, "{}", string(backend.entries()[1].Diff))

	// The write rolled back.
	resp, err := client.GetChatSystemPrompt(ctx)
	require.NoError(t, err)
	require.Equal(t, "Baseline prompt.", resp.SystemPrompt)
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
