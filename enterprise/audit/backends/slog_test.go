package backends_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/fatih/structs"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogjson"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/enterprise/audit/audittest"
	"github.com/coder/coder/v2/enterprise/audit/backends"
)

func TestSlogBackend(t *testing.T) {
	t.Parallel()
	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithCancel(context.Background())

			sink    = &fakeSink{}
			logger  = slog.Make(sink)
			backend = backends.NewSlog(logger)

			alog = audittest.RandomLog()
		)
		defer cancel()

		err := backend.Export(ctx, alog, audit.BackendDetails{})
		require.NoError(t, err)
		require.Len(t, sink.entries, 1)
		require.Equal(t, sink.entries[0].Message, "audit_log")
		require.Len(t, sink.entries[0].Fields, len(structs.Fields(alog)))
	})

	t.Run("FormatsCorrectly", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithCancel(context.Background())

			buf     = bytes.NewBuffer(nil)
			logger  = slog.Make(slogjson.Sink(buf))
			backend = backends.NewSlog(logger)

			_, inet, _ = net.ParseCIDR("127.0.0.1/32")
			alog       = database.AuditLog{
				ID:             uuid.UUID{1},
				Time:           time.Unix(1257894000, 0).UTC(),
				UserID:         uuid.UUID{2},
				OrganizationID: uuid.UUID{3},
				Ip: pqtype.Inet{
					IPNet: *inet,
					Valid: true,
				},
				UserAgent:        sql.NullString{String: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/100.0.4896.127 Safari/537.36", Valid: true},
				ResourceType:     database.ResourceTypeOrganization,
				ResourceID:       uuid.UUID{4},
				ResourceTarget:   "colin's organization",
				ResourceIcon:     "photo.png",
				Action:           database.AuditActionDelete,
				Diff:             []byte(`{"1": 2}`),
				StatusCode:       http.StatusNoContent,
				AdditionalFields: []byte(`{"name":"doug","species":"cat"}`),
				RequestID:        uuid.UUID{5},
			}
		)
		defer cancel()

		err := backend.Export(ctx, alog, audit.BackendDetails{Actor: &audit.Actor{
			ID:       uuid.UUID{2},
			Username: "coadler",
			Email:    "doug@coder.com",
		}})
		require.NoError(t, err)
		logger.Sync()

		s := struct {
			Fields json.RawMessage `json:"fields"`
		}{}
		err = json.Unmarshal(buf.Bytes(), &s)
		require.NoError(t, err)

		expected := `{"ID":"01000000-0000-0000-0000-000000000000","Time":"2009-11-10T23:00:00Z","UserID":"02000000-0000-0000-0000-000000000000","OrganizationID":"03000000-0000-0000-0000-000000000000","Ip":"127.0.0.1","UserAgent":"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/100.0.4896.127 Safari/537.36","ResourceType":"organization","ResourceID":"04000000-0000-0000-0000-000000000000","ResourceTarget":"colin's organization","Action":"delete","Diff":{"1":2},"StatusCode":204,"AdditionalFields":{"name":"doug","species":"cat"},"RequestID":"05000000-0000-0000-0000-000000000000","ResourceIcon":"photo.png","actor":{"id":"02000000-0000-0000-0000-000000000000","email":"doug@coder.com","username":"coadler"}}`
		assert.Equal(t, expected, string(s.Fields))
	})
}

type fakeSink struct {
	entries []slog.SinkEntry
}

func (s *fakeSink) LogEntry(_ context.Context, e slog.SinkEntry) {
	s.entries = append(s.entries, e)
}

func (*fakeSink) Sync() {}
