package audit_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tabbed/pqtype"

	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/database"
)

func TestExporter(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name            string
		filterDecision  audit.FilterDecision
		backendDecision audit.FilterDecision
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
		// When more filters are written they should have their own tests.
		{
			name: "DefaultFilter",
			filterDecision: func() audit.FilterDecision {
				decision, _ := audit.DefaultFilter.Check(context.Background(), randomAuditLog())
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
				backend  = &testBackend{decision: test.backendDecision}
				exporter = audit.NewExporter(
					audit.FilterFunc(func(_ context.Context, _ database.AuditLog) (audit.FilterDecision, error) {
						return test.filterDecision, nil
					}),
					backend,
				)
			)

			err := exporter.Export(context.Background(), randomAuditLog())
			require.NoError(t, err)
			require.Equal(t, len(backend.alogs) > 0, test.shouldExport)
		})
	}
}

func randomAuditLog() database.AuditLog {
	_, inet, _ := net.ParseCIDR("127.0.0.1/32")
	return database.AuditLog{
		ID:             uuid.New(),
		Time:           time.Now(),
		UserID:         uuid.New(),
		OrganizationID: uuid.New(),
		Ip: pqtype.Inet{
			IPNet: *inet,
			Valid: true,
		},
		UserAgent:      "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/100.0.4896.127 Safari/537.36",
		ResourceType:   database.ResourceTypeOrganization,
		ResourceID:     uuid.New(),
		ResourceTarget: "colin's organization",
		Action:         database.AuditActionDelete,
		Diff:           []byte{},
		StatusCode:     http.StatusNoContent,
	}
}

type testBackend struct {
	decision audit.FilterDecision

	alogs []database.AuditLog
}

func (t *testBackend) Decision() audit.FilterDecision {
	return t.decision
}

func (t *testBackend) Export(_ context.Context, alog database.AuditLog) error {
	t.alogs = append(t.alogs, alog)
	return nil
}
