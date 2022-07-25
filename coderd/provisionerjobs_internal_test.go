package coderd

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/codersdk"
)

func TestProvisionerJobLogs_Unit(t *testing.T) {
	t.Parallel()

	t.Run("QueryPubSubDupes", func(t *testing.T) {
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		// mDB := mocks.NewStore(t)
		fDB := databasefake.New()
		fPubsub := &fakePubSub{t: t, cond: sync.NewCond(&sync.Mutex{})}
		opts := Options{
			Logger:   logger,
			Database: fDB,
			Pubsub:   fPubsub,
		}
		api := New(&opts)
		server := httptest.NewServer(api.Handler)
		defer server.Close()
		userID := uuid.New()
		keyID, keySecret, err := generateAPIKeyIDSecret()
		require.NoError(t, err)
		hashed := sha256.Sum256([]byte(keySecret))

		u, err := url.Parse(server.URL)
		require.NoError(t, err)
		client := codersdk.Client{
			HTTPClient:   server.Client(),
			SessionToken: keyID + "-" + keySecret,
			URL:          u,
		}

		buildID := uuid.New()
		workspaceID := uuid.New()
		jobID := uuid.New()

		expectedLogs := []database.ProvisionerJobLog{
			{ID: uuid.New(), JobID: jobID, Stage: "Stage0"},
			{ID: uuid.New(), JobID: jobID, Stage: "Stage1"},
			{ID: uuid.New(), JobID: jobID, Stage: "Stage2"},
			{ID: uuid.New(), JobID: jobID, Stage: "Stage3"},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// wow there are a lot of DB rows we touch...
		_, err = fDB.InsertAPIKey(ctx, database.InsertAPIKeyParams{
			ID:           keyID,
			HashedSecret: hashed[:],
			UserID:       userID,
			ExpiresAt:    time.Now().Add(5 * time.Hour),
		})
		require.NoError(t, err)
		_, err = fDB.InsertUser(ctx, database.InsertUserParams{
			ID:        userID,
			RBACRoles: []string{"admin"},
		})
		require.NoError(t, err)
		_, err = fDB.InsertWorkspaceBuild(ctx, database.InsertWorkspaceBuildParams{
			ID:          buildID,
			WorkspaceID: workspaceID,
			JobID:       jobID,
		})
		require.NoError(t, err)
		_, err = fDB.InsertWorkspace(ctx, database.InsertWorkspaceParams{
			ID: workspaceID,
		})
		require.NoError(t, err)
		_, err = fDB.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID: jobID,
		})
		require.NoError(t, err)
		for _, l := range expectedLogs[:2] {
			_, err := fDB.InsertProvisionerJobLogs(ctx, database.InsertProvisionerJobLogsParams{
				ID:    []uuid.UUID{l.ID},
				JobID: jobID,
				Stage: []string{l.Stage},
			})
			require.NoError(t, err)
		}

		logs, err := client.WorkspaceBuildLogsAfter(ctx, buildID, time.Now())
		require.NoError(t, err)

		// when the endpoint calls subscribe, we get the listener here.
		fPubsub.cond.L.Lock()
		for fPubsub.listener == nil {
			fPubsub.cond.Wait()
		}

		// endpoint should now be listening
		assert.False(t, fPubsub.canceled)
		assert.False(t, fPubsub.closed)

		// send all the logs in two batches, duplicating what we already returned on the DB query.
		msg := provisionerJobLogsMessage{}
		msg.Logs = expectedLogs[:2]
		data, err := json.Marshal(msg)
		require.NoError(t, err)
		fPubsub.listener(ctx, data)
		msg.Logs = expectedLogs[2:]
		data, err = json.Marshal(msg)
		require.NoError(t, err)
		fPubsub.listener(ctx, data)

		// send end of logs
		msg.Logs = nil
		msg.EndOfLogs = true
		data, err = json.Marshal(msg)
		require.NoError(t, err)
		fPubsub.listener(ctx, data)

		var stages []string
		for l := range logs {
			logger.Info(ctx, "got log",
				slog.F("id", l.ID),
				slog.F("stage", l.Stage))
			stages = append(stages, l.Stage)
		}
		assert.Equal(t, []string{"Stage0", "Stage1", "Stage2", "Stage3"}, stages)
		for !fPubsub.canceled {
			fPubsub.cond.Wait()
		}
		assert.False(t, fPubsub.closed)
	})
}

func TestConvertProvisionerJob_Unit(t *testing.T) {
	t.Parallel()
	validNullTimeMock := sql.NullTime{
		Time:  database.Now(),
		Valid: true,
	}
	invalidNullTimeMock := sql.NullTime{}
	errorMock := sql.NullString{
		String: "error",
		Valid:  true,
	}
	testCases := []struct {
		name     string
		input    database.ProvisionerJob
		expected codersdk.ProvisionerJob
	}{
		{
			name:  "empty",
			input: database.ProvisionerJob{},
			expected: codersdk.ProvisionerJob{
				Status: codersdk.ProvisionerJobPending,
			},
		},
		{
			name: "cancellation pending",
			input: database.ProvisionerJob{
				CanceledAt:  validNullTimeMock,
				CompletedAt: invalidNullTimeMock,
			},
			expected: codersdk.ProvisionerJob{
				Status: codersdk.ProvisionerJobCanceling,
			},
		},
		{
			name: "cancellation failed",
			input: database.ProvisionerJob{
				CanceledAt:  validNullTimeMock,
				CompletedAt: validNullTimeMock,
				Error:       errorMock,
			},
			expected: codersdk.ProvisionerJob{
				CompletedAt: &validNullTimeMock.Time,
				Status:      codersdk.ProvisionerJobFailed,
				Error:       errorMock.String,
			},
		},
		{
			name: "cancellation succeeded",
			input: database.ProvisionerJob{
				CanceledAt:  validNullTimeMock,
				CompletedAt: validNullTimeMock,
			},
			expected: codersdk.ProvisionerJob{
				CompletedAt: &validNullTimeMock.Time,
				Status:      codersdk.ProvisionerJobCanceled,
			},
		},
		{
			name: "job pending",
			input: database.ProvisionerJob{
				StartedAt: invalidNullTimeMock,
			},
			expected: codersdk.ProvisionerJob{
				Status: codersdk.ProvisionerJobPending,
			},
		},
		{
			name: "job failed",
			input: database.ProvisionerJob{
				CompletedAt: validNullTimeMock,
				StartedAt:   validNullTimeMock,
				Error:       errorMock,
			},
			expected: codersdk.ProvisionerJob{
				CompletedAt: &validNullTimeMock.Time,
				StartedAt:   &validNullTimeMock.Time,
				Error:       errorMock.String,
				Status:      codersdk.ProvisionerJobFailed,
			},
		},
		{
			name: "job succeeded",
			input: database.ProvisionerJob{
				CompletedAt: validNullTimeMock,
				StartedAt:   validNullTimeMock,
			},
			expected: codersdk.ProvisionerJob{
				CompletedAt: &validNullTimeMock.Time,
				StartedAt:   &validNullTimeMock.Time,
				Status:      codersdk.ProvisionerJobSucceeded,
			},
		},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			actual := convertProvisionerJob(testCase.input)
			assert.Equal(t, testCase.expected, actual)
		})
	}
}

type fakePubSub struct {
	t        *testing.T
	cond     *sync.Cond
	listener database.Listener
	canceled bool
	closed   bool
}

func (f *fakePubSub) Subscribe(_ string, listener database.Listener) (cancel func(), err error) {
	f.cond.L.Lock()
	defer f.cond.L.Unlock()
	f.listener = listener
	f.cond.Signal()
	return f.cancel, nil
}

func (f *fakePubSub) Publish(_ string, _ []byte) error {
	f.t.Fail()
	return nil
}

func (f *fakePubSub) Close() error {
	f.cond.L.Lock()
	defer f.cond.L.Unlock()
	f.closed = true
	f.cond.Signal()
	return nil
}

func (f *fakePubSub) cancel() {
	f.cond.L.Lock()
	defer f.cond.L.Unlock()
	f.canceled = true
	f.cond.Signal()
}
