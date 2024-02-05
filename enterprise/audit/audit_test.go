package audit_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/enterprise/audit/audittest"
)

func TestAuditor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		filterDecision  audit.FilterDecision
		filterError     error
		backendDecision audit.FilterDecision
		backendError    error
		shouldExport    bool
	}{
		{
			name:            "ShouldDrop",
			filterDecision:  audit.FilterDecisionDrop,
			backendDecision: audit.FilterDecisionStore,
			shouldExport:    false,
		},
		{
			name:            "ShouldStore",
			filterDecision:  audit.FilterDecisionStore,
			backendDecision: audit.FilterDecisionStore,
			shouldExport:    true,
		},
		{
			name:            "ShouldNotStore",
			filterDecision:  audit.FilterDecisionExport,
			backendDecision: audit.FilterDecisionStore,
			shouldExport:    false,
		},
		{
			name:            "ShouldExport",
			filterDecision:  audit.FilterDecisionExport,
			backendDecision: audit.FilterDecisionExport,
			shouldExport:    true,
		},
		{
			name:            "ShouldNotExport",
			filterDecision:  audit.FilterDecisionStore,
			backendDecision: audit.FilterDecisionExport,
			shouldExport:    false,
		},
		{
			name:            "ShouldStoreOrExport",
			filterDecision:  audit.FilterDecisionStore | audit.FilterDecisionExport,
			backendDecision: audit.FilterDecisionExport,
			shouldExport:    true,
		},
		{
			name:            "FilterError",
			filterError:     xerrors.New("filter errored"),
			backendDecision: audit.FilterDecisionExport,
			shouldExport:    false,
		},
		{
			name:         "BackendError",
			backendError: xerrors.New("backend errored"),
			shouldExport: false,
		},
		// When more filters are written they should have their own tests.
		{
			name: "DefaultFilter",
			filterDecision: func() audit.FilterDecision {
				decision, _ := audit.DefaultFilter.Check(context.Background(), audittest.RandomLog())
				return decision
			}(),
			backendDecision: audit.FilterDecisionExport,
			shouldExport:    true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var (
				backend  = &testBackend{decision: test.backendDecision, err: test.backendError}
				exporter = audit.NewAuditor(
					dbmem.New(),
					audit.FilterFunc(func(_ context.Context, _ database.AuditLog) (audit.FilterDecision, error) {
						return test.filterDecision, test.filterError
					}),
					backend,
				)
			)

			err := exporter.Export(context.Background(), audittest.RandomLog())
			if test.filterError != nil {
				require.ErrorIs(t, err, test.filterError)
			} else if test.backendError != nil {
				require.ErrorIs(t, err, test.backendError)
			}

			require.Equal(t, len(backend.alogs) > 0, test.shouldExport)
		})
	}
}

type testBackend struct {
	decision audit.FilterDecision
	err      error

	alogs []testExport
}

type testExport struct {
	alog    database.AuditLog
	details audit.BackendDetails
}

func (t *testBackend) Decision() audit.FilterDecision {
	return t.decision
}

func (t *testBackend) Export(_ context.Context, alog database.AuditLog, details audit.BackendDetails) error {
	if t.err != nil {
		return t.err
	}

	t.alogs = append(t.alogs, testExport{
		alog:    alog,
		details: details,
	})
	return nil
}
