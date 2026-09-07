package chatd

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/testutil"
)

type admissionFixture struct {
	db          database.Store
	ps          pubsub.Pubsub
	owner       database.User
	org         database.Organization
	modelConfig database.ChatModelConfig
}

func newAdmissionFixture(t *testing.T) admissionFixture {
	t.Helper()
	db, ps := dbtestutil.NewDB(t)
	owner := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	return admissionFixture{db: db, ps: ps, owner: owner, org: org, modelConfig: modelConfig}
}

func (f admissionFixture) chat(t *testing.T, seed database.Chat) database.Chat {
	t.Helper()
	seed.OwnerID = f.owner.ID
	seed.OrganizationID = f.org.ID
	seed.LastModelConfigID = f.modelConfig.ID
	if seed.Status == "" {
		seed.Status = database.ChatStatusRunning
	}
	return dbgen.Chat(t, f.db, seed)
}

func (f admissionFixture) occupy(t *testing.T, chatID uuid.UUID) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	machine := chatstate.NewChatMachine(f.db, f.ps, chatID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.Acquire(chatstate.AcquireInput{WorkerID: uuid.New(), RunnerID: uuid.New()})
		return err
	}))
}

func (f admissionFixture) occupiedRoot(t *testing.T) database.Chat {
	t.Helper()
	chat := f.chat(t, database.Chat{})
	f.occupy(t, chat.ID)
	return chat
}

func (f admissionFixture) occupiedSubagent(t *testing.T, root database.Chat) database.Chat {
	t.Helper()
	chat := f.chat(t, database.Chat{
		ParentChatID: uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:   uuid.NullUUID{UUID: root.ID, Valid: true},
	})
	f.occupy(t, chat.ID)
	return chat
}

func testAdmission() *agentCapacityLimiter {
	a := newAgentCapacityLimiter(nil, 30)
	a.rootCapacity = 2
	a.subagentCapacity = 2
	return a
}

func TestAdmission_RootPoolCap(t *testing.T) {
	t.Parallel()
	f := newAdmissionFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	a := testAdmission()

	root := f.occupiedRoot(t)
	f.occupiedRoot(t)

	admitted, err := a.Admit(ctx, f.db, f.chat(t, database.Chat{}))
	require.NoError(t, err)
	require.False(t, admitted, "third root must be refused at capacity 2")

	subagent := f.chat(t, database.Chat{
		ParentChatID: uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:   uuid.NullUUID{UUID: root.ID, Valid: true},
	})
	admitted, err = a.Admit(ctx, f.db, subagent)
	require.NoError(t, err)
	require.True(t, admitted)
}

func TestAdmission_SubagentPoolCap(t *testing.T) {
	t.Parallel()
	f := newAdmissionFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	a := testAdmission()

	root := f.occupiedRoot(t)
	f.occupiedSubagent(t, root)
	f.occupiedSubagent(t, root)

	subagent := f.chat(t, database.Chat{
		ParentChatID: uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:   uuid.NullUUID{UUID: root.ID, Valid: true},
	})
	admitted, err := a.Admit(ctx, f.db, subagent)
	require.NoError(t, err)
	require.False(t, admitted, "third subagent must be refused at capacity 2")

	admitted, err = a.Admit(ctx, f.db, f.chat(t, database.Chat{}))
	require.NoError(t, err)
	require.True(t, admitted, "a full subagent pool must not refuse roots")
}

func TestAdmission_InterruptingBypassesCap(t *testing.T) {
	t.Parallel()
	f := newAdmissionFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	a := testAdmission()

	f.occupiedRoot(t)
	f.occupiedRoot(t)

	interrupting := f.chat(t, database.Chat{Status: database.ChatStatusInterrupting})
	admitted, err := a.Admit(ctx, f.db, interrupting)
	require.NoError(t, err)
	require.True(t, admitted, "interrupting chats must always be acquirable")
}

func TestAdmission_RequiresActionBypassesCap(t *testing.T) {
	t.Parallel()
	f := newAdmissionFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	a := testAdmission()

	f.occupiedRoot(t)
	f.occupiedRoot(t)

	requiresAction := f.chat(t, database.Chat{Status: database.ChatStatusRequiresAction})
	admitted, err := a.Admit(ctx, f.db, requiresAction)
	require.NoError(t, err)
	require.True(t, admitted, "requires_action chats hold no slot and need their runner")
}

func TestAdmission_TakeoverOfCountedChatIsCapacityNeutral(t *testing.T) {
	t.Parallel()
	f := newAdmissionFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	a := testAdmission()

	counted := f.occupiedRoot(t)
	f.occupiedRoot(t)

	chat, err := f.db.GetChatByID(ctx, counted.ID)
	require.NoError(t, err)
	admitted, err := a.Admit(ctx, f.db, chat)
	require.NoError(t, err)
	require.True(t, admitted, "an already-counted chat must re-admit for takeover at full capacity")
}

// The single transaction verifies that the admission lock covers the
// ownership write.
func TestAdmission_ConcurrentAdmitNeverOverAdmits(t *testing.T) {
	t.Parallel()
	f := newAdmissionFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	a := testAdmission()

	const attempts = 8
	chats := make([]database.Chat, attempts)
	for i := range chats {
		chats[i] = f.chat(t, database.Chat{})
	}

	errRefused := xerrors.New("refused")
	var (
		admitted   atomic.Int64
		unexpected atomic.Int64
		wg         sync.WaitGroup
	)
	for _, chat := range chats {
		wg.Go(func() {
			machine := chatstate.NewChatMachine(f.db, f.ps, chat.ID)
			err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
				ok, err := a.Admit(ctx, store, chat)
				if err != nil {
					return err
				}
				if !ok {
					return errRefused
				}
				_, err = tx.Acquire(chatstate.AcquireInput{WorkerID: uuid.New(), RunnerID: uuid.New()})
				return err
			})
			switch {
			case err == nil:
				admitted.Add(1)
			case errors.Is(err, errRefused):
			default:
				unexpected.Add(1)
			}
		})
	}
	wg.Wait()

	require.EqualValues(t, 0, unexpected.Load(), "admission attempts must not error")
	require.EqualValues(t, 2, admitted.Load(), "exactly rootCapacity chats must admit")
}

func TestAdmission_StaleHeartbeatsFreeSlots(t *testing.T) {
	t.Parallel()
	f := newAdmissionFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	f.occupiedRoot(t)
	f.occupiedRoot(t)

	// A zero staleness window makes every heartbeat stale.
	a := newAgentCapacityLimiter(nil, 0)
	a.rootCapacity = 2
	a.subagentCapacity = 2
	admitted, err := a.Admit(ctx, f.db, f.chat(t, database.Chat{}))
	require.NoError(t, err)
	require.True(t, admitted)
}

type staticAgentCapacityUnlock bool

func (u staticAgentCapacityUnlock) Unlocked() bool {
	return bool(u)
}

func TestAdmission_UnlockBypassesCaps(t *testing.T) {
	t.Parallel()
	a := newAgentCapacityLimiter(staticAgentCapacityUnlock(true), 30)

	admitted, err := a.Admit(t.Context(), nil, database.Chat{Status: database.ChatStatusRunning})
	require.NoError(t, err)
	require.True(t, admitted)

	_, capped := a.Limits()
	require.False(t, capped)
}

func TestAdmission_LimitsReportsCaps(t *testing.T) {
	t.Parallel()
	a := newAgentCapacityLimiter(nil, 30)

	limits, capped := a.Limits()
	require.True(t, capped)
	require.EqualValues(t, defaultMaxConcurrentRootAgents, limits.Root)
	require.EqualValues(t, defaultMaxConcurrentSubagents, limits.Subagent)
}
