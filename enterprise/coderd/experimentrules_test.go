package coderd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/experiments"
	"github.com/coder/coder/v2/codersdk"
	entaudit "github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/enterprise/audit/backends"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/support"
	"github.com/coder/coder/v2/testutil"
)

type experimentRulesFixture struct {
	db         database.Store
	owner      *codersdk.Client
	ownerID    uuid.UUID
	orgID      uuid.UUID
	client     *codersdk.ExperimentalClient
	logs       *testutil.FakeSink
	failUpsert *atomic.Bool
}

// newExperimentRulesFixture starts a licensed deployment that audits into
// the database, plus extra audit backends built with the server logger,
// and captures server logs.
func newExperimentRulesFixture(t *testing.T, extraBackends ...func(slog.Logger) entaudit.Backend) experimentRulesFixture {
	t.Helper()

	db, ps := dbtestutil.NewDB(t)
	failUpsert := &atomic.Bool{}
	logs := testutil.NewFakeSink(t)
	logger := logs.Logger()
	auditBackends := []entaudit.Backend{backends.NewPostgres(db, true)}
	for _, backend := range extraBackends {
		auditBackends = append(auditBackends, backend(logger))
	}
	auditor := entaudit.NewAuditor(db, entaudit.DefaultFilter, auditBackends...)
	ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
		AuditLogging: true,
		Options: &coderdtest.Options{
			Database: upsertFaultStore{Store: db, fail: failUpsert},
			Pubsub:   ps,
			Auditor:  auditor,
			Logger:   &logger,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{codersdk.FeatureAuditLog: 1},
		},
	})
	return experimentRulesFixture{
		db:         db,
		owner:      ownerClient,
		ownerID:    owner.UserID,
		orgID:      owner.OrganizationID,
		client:     codersdk.NewExperimentalClient(ownerClient),
		logs:       logs,
		failUpsert: failUpsert,
	}
}

func (f experimentRulesFixture) auditLogs(ctx context.Context, t *testing.T) []database.AuditLog {
	t.Helper()
	rows, err := f.db.GetAuditLogsOffset(dbauthz.AsSystemRestricted(ctx), database.GetAuditLogsOffsetParams{
		ResourceType: string(database.ResourceTypeExperimentRule),
		LimitOpt:     50,
	})
	require.NoError(t, err)
	logs := make([]database.AuditLog, 0, len(rows))
	for _, row := range rows {
		logs = append(logs, row.AuditLog)
	}
	return logs
}

// waitAuditLogs waits until n experiment rule audit entries exist. Entries
// are exported after the response is written, so they can trail it.
func (f experimentRulesFixture) waitAuditLogs(ctx context.Context, t *testing.T, n int) []database.AuditLog {
	t.Helper()
	var logs []database.AuditLog
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		logs = f.auditLogs(ctx, t)
		return len(logs) >= n
	}, testutil.IntervalFast)
	require.Len(t, logs, n)
	return logs
}

func (f experimentRulesFixture) put(ctx context.Context, mode codersdk.ExperimentRuleMode, condition string, expected int64) (codersdk.ExperimentRule, error) {
	return f.client.PutExperimentRule(ctx, codersdk.ExperimentExample, codersdk.PutExperimentRuleRequest{
		Mode: mode, Condition: condition, ExpectedRevision: expected,
	})
}

func (f experimentRulesFixture) storedRule(ctx context.Context, t *testing.T) *codersdk.ExperimentRule {
	t.Helper()
	entries, err := f.client.ExperimentRules(ctx)
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.Experiment == string(codersdk.ExperimentExample) {
			return entry.Rule
		}
	}
	t.Fatal("no rules entry for the example experiment")
	return nil
}

func requireStatusCode(t *testing.T, err error, status int) {
	t.Helper()
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, status, sdkErr.StatusCode(), sdkErr.Error())
}

func TestExperimentRuleAudit(t *testing.T) {
	t.Parallel()

	t.Run("WritePath", func(t *testing.T) {
		t.Parallel()
		f := newExperimentRulesFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		const condition = `user.username == "audited-condition"`

		stored, err := f.put(ctx, codersdk.ExperimentRuleModeCondition, condition, 0)
		require.NoError(t, err)

		// Require the entry before inspecting it, so the diff checks
		// cannot pass because nothing was audited.
		entry := f.waitAuditLogs(ctx, t, 1)[0]
		require.Equal(t, int32(http.StatusOK), entry.StatusCode)
		require.Equal(t, database.AuditActionWrite, entry.Action)
		require.Equal(t, string(codersdk.ExperimentExample), entry.ResourceTarget)
		require.Equal(t, experiments.AuditRecord(codersdk.ExperimentExample, experiments.Rule{}).ID, entry.ResourceID)
		require.Equal(t, f.ownerID, entry.UserID)
		var diff audit.Map
		require.NoError(t, json.Unmarshal(entry.Diff, &diff))
		require.Equal(t, audit.OldNew{Old: "", New: condition}, diff["condition"])
		require.Equal(t, audit.OldNew{Old: "", New: "condition"}, diff["mode"])
		require.Equal(t, audit.OldNew{Old: float64(0), New: float64(1)}, diff["revision"])

		// An identical write is a no-op: 200 with the unchanged rule and
		// no entry. The conflict below is audited after it, so a stray
		// no-op entry would show up in the count.
		same, err := f.put(ctx, codersdk.ExperimentRuleModeCondition, condition, 1)
		require.NoError(t, err)
		require.Equal(t, stored, same)

		// Denied and invalid writes are rejected before the audit starts,
		// so they add no entry to the count either.
		memberClient, _ := coderdtest.CreateAnotherUser(t, f.owner, f.orgID)
		_, err = codersdk.NewExperimentalClient(memberClient).PutExperimentRule(ctx, codersdk.ExperimentExample, codersdk.PutExperimentRuleRequest{Mode: codersdk.ExperimentRuleModeOff, ExpectedRevision: 1})
		requireStatusCode(t, err, http.StatusForbidden)
		_, err = f.put(ctx, codersdk.ExperimentRuleModeCondition, "user.", 1)
		requireStatusCode(t, err, http.StatusBadRequest)

		_, err = f.put(ctx, codersdk.ExperimentRuleModeOff, "", 0)
		requireStatusCode(t, err, http.StatusConflict)
		conflict := f.waitAuditLogs(ctx, t, 2)[0]
		require.Equal(t, int32(http.StatusConflict), conflict.StatusCode)
		require.JSONEq(t, "{}", string(conflict.Diff))

		// A database error inside the transaction rolls the write back
		// and is audited with its status.
		f.failUpsert.Store(true)
		_, err = f.put(ctx, codersdk.ExperimentRuleModeOff, "", 1)
		requireStatusCode(t, err, http.StatusInternalServerError)
		f.failUpsert.Store(false)
		failed := f.waitAuditLogs(ctx, t, 3)[0]
		require.Equal(t, int32(http.StatusInternalServerError), failed.StatusCode)
		require.JSONEq(t, "{}", string(failed.Diff))
		require.Equal(t, &stored, f.storedRule(ctx, t))
	})

	// The audit entry is exported after the rule commits. When the export
	// fails, the change stands and the failure is logged.
	t.Run("FailedExportKeepsChange", func(t *testing.T) {
		t.Parallel()
		f := newExperimentRulesFixture(t, func(slog.Logger) entaudit.Backend { return failingAuditBackend{} })
		ctx := testutil.Context(t, testutil.WaitLong)

		stored, err := f.put(ctx, codersdk.ExperimentRuleModeOn, "", 0)
		require.NoError(t, err)
		testutil.Eventually(ctx, t, func(context.Context) bool {
			return len(f.logs.Entries(func(e slog.SinkEntry) bool {
				return e.Level == slog.LevelError && e.Message == "export audit log"
			})) > 0
		}, testutil.IntervalFast)
		require.Equal(t, &stored, f.storedRule(ctx, t))
	})
}

// TestExperimentRuleRedaction checks where condition text can appear. The
// rules API, audit entries and the server log's audit lines carry it. No
// other log line, /deployment/config or a support bundle may, even for
// invalid conditions and conditions that fail during evaluation.
func TestExperimentRuleRedaction(t *testing.T) {
	t.Parallel()
	// Enterprise servers always export audit entries to the server log.
	f := newExperimentRulesFixture(t, func(logger slog.Logger) entaudit.Backend { return backends.NewSlog(logger) })
	memberClient, _ := coderdtest.CreateAnotherUser(t, f.owner, f.orgID)
	ctx := testutil.Context(t, testutil.WaitLong)
	sentinel := "redaction-sentinel-" + uuid.NewString()

	// The author of an invalid condition gets diagnostics quoting it.
	_, err := f.put(ctx, codersdk.ExperimentRuleModeCondition, fmt.Sprintf("user.email == %q &&", sentinel), 0)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	require.Contains(t, sdkErr.Detail, sentinel)

	// A valid condition, then one that fails while the member's
	// experiments are evaluated.
	validCondition := fmt.Sprintf("user.email == %q", sentinel)
	failingCondition := fmt.Sprintf("user.groups[7] == %q", sentinel)
	_, err = f.put(ctx, codersdk.ExperimentRuleModeCondition, validCondition, 0)
	require.NoError(t, err)
	_, err = memberClient.Experiments(ctx)
	require.NoError(t, err)
	_, err = f.put(ctx, codersdk.ExperimentRuleModeCondition, failingCondition, 1)
	require.NoError(t, err)
	_, err = memberClient.Experiments(ctx)
	require.NoError(t, err)
	testutil.Eventually(ctx, t, func(context.Context) bool {
		return len(f.logs.Entries(func(e slog.SinkEntry) bool {
			return e.Message == "experiment condition failed; experiment is off"
		})) > 0
	}, testutil.IntervalFast)
	require.Contains(t, f.storedRule(ctx, t).Condition, sentinel)

	// The invalid write is not audited. The newest entry records the
	// condition change with both texts.
	latest := f.waitAuditLogs(ctx, t, 2)[0]
	var diff audit.Map
	require.NoError(t, json.Unmarshal(latest.Diff, &diff))
	require.Equal(t, audit.OldNew{Old: validCondition, New: failingCondition}, diff["condition"])

	res, err := f.owner.Request(ctx, http.MethodGet, "/api/v2/deployment/config", nil)
	require.NoError(t, err)
	body, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.NotContains(t, string(body), sentinel)

	bundle, err := support.Run(ctx, &support.Deps{Client: f.owner, Log: slog.Make()})
	require.NoError(t, err)
	bundleJSON, err := json.Marshal(bundle)
	require.NoError(t, err)
	require.NotContains(t, string(bundleJSON), sentinel)

	// Audit lines are exported after the handler returns. Requiring one
	// with the condition keeps the check below from passing vacuously.
	auditLine := func(e slog.SinkEntry) bool { return e.Message == "audit_log" || e.Message == "export audit log" }
	testutil.Eventually(ctx, t, func(context.Context) bool {
		for _, entry := range f.logs.Entries(func(e slog.SinkEntry) bool { return e.Message == "audit_log" }) {
			if strings.Contains(logText(entry), sentinel) {
				return true
			}
		}
		return false
	}, testutil.IntervalFast)
	for _, entry := range f.logs.Entries(func(e slog.SinkEntry) bool { return !auditLine(e) }) {
		require.False(t, strings.Contains(logText(entry), sentinel), "log entry contains condition text: %s", entry.Message)
	}
}

// logText renders a log entry's message and field values. Byte slices,
// such as audit diffs, are rendered as text.
func logText(entry slog.SinkEntry) string {
	var b strings.Builder
	b.WriteString(entry.Message)
	for _, field := range entry.Fields {
		b.WriteString(" ")
		switch v := field.Value.(type) {
		case json.RawMessage:
			b.Write(v)
		case []byte:
			b.Write(v)
		default:
			_, _ = fmt.Fprintf(&b, "%+v", v)
		}
	}
	return b.String()
}

// upsertFaultStore fails experiment rule upserts while fail is set.
type upsertFaultStore struct {
	database.Store
	fail *atomic.Bool
}

func (s upsertFaultStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error {
		return fn(upsertFaultStore{Store: tx, fail: s.fail})
	}, opts)
}

func (s upsertFaultStore) UpsertExperimentRule(ctx context.Context, arg database.UpsertExperimentRuleParams) error {
	if s.fail.Load() {
		return xerrors.New("injected upsert failure")
	}
	return s.Store.UpsertExperimentRule(ctx, arg)
}

// failingAuditBackend fails every export.
type failingAuditBackend struct{}

func (failingAuditBackend) Decision() entaudit.FilterDecision { return entaudit.FilterDecisionStore }

func (failingAuditBackend) Export(context.Context, database.AuditLog, entaudit.BackendDetails) error {
	return xerrors.New("injected audit export failure")
}
