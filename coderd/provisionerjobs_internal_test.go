package coderd

import (
	"database/sql"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
)

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
