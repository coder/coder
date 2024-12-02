package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

func TestConvertProvisionerJob_Unit(t *testing.T) {
	t.Parallel()
	validNullTimeMock := sql.NullTime{
		Time:  dbtime.Now(),
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
			name: "empty",
			input: database.ProvisionerJob{
				JobStatus: database.ProvisionerJobStatusPending,
			},
			expected: codersdk.ProvisionerJob{
				Status: codersdk.ProvisionerJobPending,
			},
		},
		{
			name: "cancellation pending",
			input: database.ProvisionerJob{
				CanceledAt:  validNullTimeMock,
				CompletedAt: invalidNullTimeMock,
				JobStatus:   database.ProvisionerJobStatusCanceling,
			},
			expected: codersdk.ProvisionerJob{
				CanceledAt: &validNullTimeMock.Time,
				Status:     codersdk.ProvisionerJobCanceling,
			},
		},
		{
			name: "cancellation failed",
			input: database.ProvisionerJob{
				CanceledAt:  validNullTimeMock,
				CompletedAt: validNullTimeMock,
				Error:       errorMock,
				JobStatus:   database.ProvisionerJobStatusFailed,
			},
			expected: codersdk.ProvisionerJob{
				CanceledAt:  &validNullTimeMock.Time,
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
				JobStatus:   database.ProvisionerJobStatusCanceled,
			},
			expected: codersdk.ProvisionerJob{
				CanceledAt:  &validNullTimeMock.Time,
				CompletedAt: &validNullTimeMock.Time,
				Status:      codersdk.ProvisionerJobCanceled,
			},
		},
		{
			name: "job pending",
			input: database.ProvisionerJob{
				StartedAt: invalidNullTimeMock,
				JobStatus: database.ProvisionerJobStatusPending,
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
				JobStatus:   database.ProvisionerJobStatusFailed,
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
				JobStatus:   database.ProvisionerJobStatusSucceeded,
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
			actual := convertProvisionerJob(database.GetProvisionerJobsByIDsWithQueuePositionRow{
				ProvisionerJob: testCase.input,
			})
			assert.Equal(t, testCase.expected, actual)
		})
	}
}

func Test_logFollower_completeBeforeFollow(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	logger := testutil.Logger(t)
	ctrl := gomock.NewController(t)
	mDB := dbmock.NewMockStore(ctrl)
	ps := pubsub.NewInMemory()
	now := dbtime.Now()
	job := database.ProvisionerJob{
		ID:        uuid.New(),
		CreatedAt: now.Add(-10 * time.Second),
		UpdatedAt: now.Add(-10 * time.Second),
		StartedAt: sql.NullTime{
			Time:  now.Add(-10 * time.Second),
			Valid: true,
		},
		CompletedAt: sql.NullTime{
			Time:  now.Add(-time.Second),
			Valid: true,
		},
		Error:     sql.NullString{},
		JobStatus: database.ProvisionerJobStatusSucceeded,
	}

	// we need an HTTP server to get a websocket
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		uut := newLogFollower(ctx, logger, mDB, ps, rw, r, job, 10)
		uut.follow()
	}))
	defer srv.Close()

	// return some historical logs
	mDB.EXPECT().GetProvisionerLogsAfterID(gomock.Any(), matchesJobAfter(job.ID, 10)).
		Times(1).
		Return(
			[]database.ProvisionerJobLog{
				{Stage: "One", Output: "One", ID: 11},
				{Stage: "One", Output: "Two", ID: 12},
			},
			nil,
		)

	// nolint: bodyclose
	client, _, err := websocket.Dial(ctx, srv.URL, nil)
	require.NoError(t, err)
	mt, msg, err := client.Read(ctx)
	require.NoError(t, err)
	assert.Equal(t, websocket.MessageText, mt)
	assertLog(t, "One", "One", 11, msg)

	mt, msg, err = client.Read(ctx)
	require.NoError(t, err)
	assert.Equal(t, websocket.MessageText, mt)
	assertLog(t, "One", "Two", 12, msg)

	// server should now close
	_, _, err = client.Read(ctx)
	var closeErr websocket.CloseError
	require.ErrorAs(t, err, &closeErr)
	assert.Equal(t, websocket.StatusNormalClosure, closeErr.Code)
}

func Test_logFollower_completeBeforeSubscribe(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	logger := testutil.Logger(t)
	ctrl := gomock.NewController(t)
	mDB := dbmock.NewMockStore(ctrl)
	ps := pubsub.NewInMemory()
	now := dbtime.Now()
	job := database.ProvisionerJob{
		ID:        uuid.New(),
		CreatedAt: now.Add(-10 * time.Second),
		UpdatedAt: now.Add(-10 * time.Second),
		StartedAt: sql.NullTime{
			Time:  now.Add(-10 * time.Second),
			Valid: true,
		},
		CanceledAt:  sql.NullTime{},
		CompletedAt: sql.NullTime{},
		Error:       sql.NullString{},
		JobStatus:   database.ProvisionerJobStatusRunning,
	}

	// we need an HTTP server to get a websocket
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		uut := newLogFollower(ctx, logger, mDB, ps, rw, r, job, 0)
		uut.follow()
	}))
	defer srv.Close()

	// job was incomplete when we create the logFollower, but is complete as soon
	// as it queries again.
	mDB.EXPECT().GetProvisionerJobByID(gomock.Any(), job.ID).Times(1).Return(
		database.ProvisionerJob{
			ID:        job.ID,
			CreatedAt: job.CreatedAt,
			UpdatedAt: job.UpdatedAt,
			StartedAt: job.StartedAt,
			CompletedAt: sql.NullTime{
				Time:  now.Add(-time.Millisecond),
				Valid: true,
			},
			JobStatus: database.ProvisionerJobStatusSucceeded,
		},
		nil,
	)

	// return some historical logs
	mDB.EXPECT().GetProvisionerLogsAfterID(gomock.Any(), matchesJobAfter(job.ID, 0)).
		Times(1).
		Return(
			[]database.ProvisionerJobLog{
				{Stage: "One", Output: "One", ID: 1},
				{Stage: "One", Output: "Two", ID: 2},
			},
			nil,
		)

	// nolint: bodyclose
	client, _, err := websocket.Dial(ctx, srv.URL, nil)
	require.NoError(t, err)
	mt, msg, err := client.Read(ctx)
	require.NoError(t, err)
	assert.Equal(t, websocket.MessageText, mt)
	assertLog(t, "One", "One", 1, msg)

	mt, msg, err = client.Read(ctx)
	require.NoError(t, err)
	assert.Equal(t, websocket.MessageText, mt)
	assertLog(t, "One", "Two", 2, msg)

	// server should now close
	_, _, err = client.Read(ctx)
	var closeErr websocket.CloseError
	require.ErrorAs(t, err, &closeErr)
	assert.Equal(t, websocket.StatusNormalClosure, closeErr.Code)
}

func Test_logFollower_EndOfLogs(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	logger := testutil.Logger(t)
	ctrl := gomock.NewController(t)
	mDB := dbmock.NewMockStore(ctrl)
	ps := pubsub.NewInMemory()
	now := dbtime.Now()
	job := database.ProvisionerJob{
		ID:        uuid.New(),
		CreatedAt: now.Add(-10 * time.Second),
		UpdatedAt: now.Add(-10 * time.Second),
		StartedAt: sql.NullTime{
			Time:  now.Add(-10 * time.Second),
			Valid: true,
		},
		CanceledAt:  sql.NullTime{},
		CompletedAt: sql.NullTime{},
		Error:       sql.NullString{},
		JobStatus:   database.ProvisionerJobStatusRunning,
	}

	// we need an HTTP server to get a websocket
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		uut := newLogFollower(ctx, logger, mDB, ps, rw, r, job, 0)
		uut.follow()
	}))
	defer srv.Close()

	// job was incomplete when we create the logFollower, and still incomplete when it queries
	mDB.EXPECT().GetProvisionerJobByID(gomock.Any(), job.ID).Times(1).Return(job, nil)

	// return some historical logs
	q0 := mDB.EXPECT().GetProvisionerLogsAfterID(gomock.Any(), matchesJobAfter(job.ID, 0)).
		Times(1).
		Return(
			[]database.ProvisionerJobLog{
				{Stage: "One", Output: "One", ID: 1},
				{Stage: "One", Output: "Two", ID: 2},
			},
			nil,
		)
	// return some logs from a kick.
	mDB.EXPECT().GetProvisionerLogsAfterID(gomock.Any(), matchesJobAfter(job.ID, 2)).
		After(q0).
		Times(1).
		Return(
			[]database.ProvisionerJobLog{
				{Stage: "One", Output: "Three", ID: 3},
				{Stage: "Two", Output: "One", ID: 4},
			},
			nil,
		)

	// nolint: bodyclose
	client, _, err := websocket.Dial(ctx, srv.URL, nil)
	require.NoError(t, err)
	mt, msg, err := client.Read(ctx)
	require.NoError(t, err)
	assert.Equal(t, websocket.MessageText, mt)
	assertLog(t, "One", "One", 1, msg)

	mt, msg, err = client.Read(ctx)
	require.NoError(t, err)
	assert.Equal(t, websocket.MessageText, mt)
	assertLog(t, "One", "Two", 2, msg)

	// send in the kick so follower will query a second time
	n := provisionersdk.ProvisionerJobLogsNotifyMessage{
		CreatedAfter: 2,
	}
	msg, err = json.Marshal(&n)
	require.NoError(t, err)
	err = ps.Publish(provisionersdk.ProvisionerJobLogsNotifyChannel(job.ID), msg)
	require.NoError(t, err)

	mt, msg, err = client.Read(ctx)
	require.NoError(t, err)
	assert.Equal(t, websocket.MessageText, mt)
	assertLog(t, "One", "Three", 3, msg)

	mt, msg, err = client.Read(ctx)
	require.NoError(t, err)
	assert.Equal(t, websocket.MessageText, mt)
	assertLog(t, "Two", "One", 4, msg)

	// send EndOfLogs
	n.EndOfLogs = true
	n.CreatedAfter = 0
	msg, err = json.Marshal(&n)
	require.NoError(t, err)
	err = ps.Publish(provisionersdk.ProvisionerJobLogsNotifyChannel(job.ID), msg)
	require.NoError(t, err)

	// server should now close
	_, _, err = client.Read(ctx)
	var closeErr websocket.CloseError
	require.ErrorAs(t, err, &closeErr)
	assert.Equal(t, websocket.StatusNormalClosure, closeErr.Code)
}

func assertLog(t *testing.T, stage, output string, id int64, msg []byte) {
	t.Helper()
	var log codersdk.ProvisionerJobLog
	err := json.Unmarshal(msg, &log)
	require.NoError(t, err)
	assert.Equal(t, stage, log.Stage)
	assert.Equal(t, output, log.Output)
	assert.Equal(t, id, log.ID)
}

type logsAfterMatcher struct {
	params database.GetProvisionerLogsAfterIDParams
}

func (m *logsAfterMatcher) Matches(x interface{}) bool {
	p, ok := x.(database.GetProvisionerLogsAfterIDParams)
	if !ok {
		return false
	}
	return m.params == p
}

func (m *logsAfterMatcher) String() string {
	return fmt.Sprintf("%+v", m.params)
}

func matchesJobAfter(jobID uuid.UUID, after int64) gomock.Matcher {
	return &logsAfterMatcher{
		params: database.GetProvisionerLogsAfterIDParams{
			JobID:        jobID,
			CreatedAfter: after,
		},
	}
}
