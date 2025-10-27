package dynamicparameters

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestPartitionEvaluations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    int
		expected []int
	}{
		{
			name:     "10",
			input:    10,
			expected: []int{5, 3, 1, 1},
		},
		{
			name:     "11",
			input:    11,
			expected: []int{6, 3, 1, 1},
		},
		{
			name:     "12",
			input:    12,
			expected: []int{6, 3, 2, 1},
		},
		{
			name:     "600",
			input:    600,
			expected: []int{300, 150, 75, 38, 19, 9, 5, 2, 1, 1},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := partitionEvaluations(tc.input)
			require.Equal(t, tc.expected, got)
			total := 0
			for _, v := range got {
				total += v
			}
			require.Equal(t, tc.input, total)
		})
	}
}

func TestSetupPartitions_TemplateExists(t *testing.T) {
	t.Parallel()
	logger := testutil.Logger(t).Leveled(slog.LevelDebug)
	ctx := testutil.Context(t, testutil.WaitShort)

	orgID := uuid.New()
	fClient := &fakeClient{
		t:                        t,
		expectedTemplateName:     "test-template",
		expectedOrgID:            orgID,
		expectedTags:             map[string]string{"foo": "bar"},
		matchedProvisioners:      1,
		templateVersionJobStatus: codersdk.ProvisionerJobSucceeded,
	}
	mClock := quartz.NewMock(t)
	trap := mClock.Trap().TickerFunc("waitForTemplateVersionJobs")
	defer trap.Close()
	uut := partitioner{
		ctx:             ctx,
		client:          fClient,
		orgID:           orgID,
		templateName:    "test-template",
		provisionerTags: map[string]string{"foo": "bar"},
		numEvals:        600,
		logger:          logger,
		clock:           mClock,
	}
	var partitions []Partition
	errCh := make(chan error, 1)
	go func() {
		var err error
		partitions, err = uut.run()
		errCh <- err
	}()
	trap.MustWait(ctx).MustRelease(ctx)
	mClock.Advance(time.Second * 2).MustWait(ctx)
	err := testutil.RequireReceive(ctx, t, errCh)
	require.NoError(t, err)
	// 600 evaluations should be partitioned into 10 parts: []int{300, 150, 75, 38, 19, 9, 5, 2, 1, 1}
	// c.f. TestPartitionEvaluations. That's 10 template versions and associated uploads.
	require.Equal(t, 10, len(partitions))
	require.Equal(t, 10, fClient.templateVersionsCount)
	require.Equal(t, 10, fClient.uploadsCount)
	require.Equal(t, 1, fClient.templateByNameCount)
	require.Equal(t, 0, fClient.createTemplateCount)
}

func TestSetupPartitions_TemplateDoesntExist(t *testing.T) {
	t.Parallel()
	logger := testutil.Logger(t).Leveled(slog.LevelDebug)
	ctx := testutil.Context(t, testutil.WaitShort)

	orgID := uuid.New()
	fClient := &fakeClient{
		t:                        t,
		expectedTemplateName:     "test-template",
		expectedOrgID:            orgID,
		templateByNameError:      codersdk.NewTestError(http.StatusNotFound, "", ""),
		matchedProvisioners:      1,
		templateVersionJobStatus: codersdk.ProvisionerJobSucceeded,
	}
	mClock := quartz.NewMock(t)
	trap := mClock.Trap().TickerFunc("waitForTemplateVersionJobs")
	defer trap.Close()
	uut := partitioner{
		ctx:          ctx,
		client:       fClient,
		orgID:        orgID,
		templateName: "test-template",
		numEvals:     600,
		logger:       logger,
		clock:        mClock,
	}
	var partitions []Partition
	errCh := make(chan error, 1)
	go func() {
		var err error
		partitions, err = uut.run()
		errCh <- err
	}()
	trap.MustWait(ctx).MustRelease(ctx)
	mClock.Advance(time.Second * 2).MustWait(ctx)
	err := testutil.RequireReceive(ctx, t, errCh)
	require.NoError(t, err)
	// 600 evaluations should be partitioned into 10 parts: []int{300, 150, 75, 38, 19, 9, 5, 2, 1, 1}
	// c.f. TestPartitionEvaluations. That's 10 template versions and associated uploads.
	require.Equal(t, 10, len(partitions))
	require.Equal(t, 10, fClient.templateVersionsCount)
	require.Equal(t, 10, fClient.uploadsCount)
	require.Equal(t, 1, fClient.templateByNameCount)
	require.Equal(t, 1, fClient.createTemplateCount)
}

func TestSetupPartitions_NoMatchedProvisioners(t *testing.T) {
	t.Parallel()
	logger := testutil.Logger(t).Leveled(slog.LevelDebug)
	ctx := testutil.Context(t, testutil.WaitShort)

	orgID := uuid.New()
	fClient := &fakeClient{
		t:                        t,
		expectedTemplateName:     "test-template",
		expectedOrgID:            orgID,
		matchedProvisioners:      0,
		templateVersionJobStatus: codersdk.ProvisionerJobSucceeded,
	}
	mClock := quartz.NewMock(t)
	uut := partitioner{
		ctx:          ctx,
		client:       fClient,
		orgID:        orgID,
		templateName: "test-template",
		numEvals:     600,
		logger:       logger,
		clock:        mClock,
	}
	errCh := make(chan error, 1)
	go func() {
		_, err := uut.run()
		errCh <- err
	}()
	err := testutil.RequireReceive(ctx, t, errCh)
	require.ErrorIs(t, err, ErrNoProvisionersMatched)
	require.Equal(t, 1, fClient.templateVersionsCount)
	require.Equal(t, 1, fClient.uploadsCount)
	require.Equal(t, 1, fClient.templateByNameCount)
	require.Equal(t, 0, fClient.createTemplateCount)
}

func TestSetupPartitions_JobFailed(t *testing.T) {
	t.Parallel()
	logger := testutil.Logger(t).Leveled(slog.LevelDebug)
	ctx := testutil.Context(t, testutil.WaitShort)

	orgID := uuid.New()
	fClient := &fakeClient{
		t:                        t,
		expectedTemplateName:     "test-template",
		expectedOrgID:            orgID,
		matchedProvisioners:      1,
		templateVersionJobStatus: codersdk.ProvisionerJobFailed,
	}
	mClock := quartz.NewMock(t)
	trap := mClock.Trap().TickerFunc("waitForTemplateVersionJobs")
	defer trap.Close()
	uut := partitioner{
		ctx:          ctx,
		client:       fClient,
		orgID:        orgID,
		templateName: "test-template",
		numEvals:     600,
		logger:       logger,
		clock:        mClock,
	}
	errCh := make(chan error, 1)
	go func() {
		_, err := uut.run()
		errCh <- err
	}()
	trap.MustWait(ctx).MustRelease(ctx)
	mClock.Advance(time.Second * 2).MustWait(ctx)
	err := testutil.RequireReceive(ctx, t, errCh)
	require.ErrorAs(t, err, &ProvisionerJobUnexpectedStatusError{})
	require.Equal(t, 10, fClient.templateVersionsCount)
	require.Equal(t, 10, fClient.uploadsCount)
	require.Equal(t, 1, fClient.templateByNameCount)
	require.Equal(t, 0, fClient.createTemplateCount)
}

type fakeClient struct {
	t testing.TB

	expectedTemplateName string
	expectedOrgID        uuid.UUID
	templateByNameError  error

	expectedTags             map[string]string
	matchedProvisioners      int
	templateVersionJobStatus codersdk.ProvisionerJobStatus

	createTemplateCount   int
	templateVersionsCount int
	uploadsCount          int
	templateByNameCount   int
}

func (f *fakeClient) TemplateByName(ctx context.Context, orgID uuid.UUID, templateName string) (codersdk.Template, error) {
	f.templateByNameCount++
	require.Equal(f.t, f.expectedOrgID, orgID)
	require.Equal(f.t, f.expectedTemplateName, templateName)

	if f.templateByNameError != nil {
		return codersdk.Template{}, f.templateByNameError
	}
	return codersdk.Template{
		ID:   uuid.New(),
		Name: f.expectedTemplateName,
	}, nil
}

func (f *fakeClient) CreateTemplate(ctx context.Context, orgID uuid.UUID, createReq codersdk.CreateTemplateRequest) (codersdk.Template, error) {
	f.createTemplateCount++
	require.Equal(f.t, f.expectedOrgID, orgID)
	require.Equal(f.t, f.expectedTemplateName, createReq.Name)

	return codersdk.Template{
		ID:   uuid.New(),
		Name: f.expectedTemplateName,
	}, nil
}

func (f *fakeClient) CreateTemplateVersion(ctx context.Context, orgID uuid.UUID, createReq codersdk.CreateTemplateVersionRequest) (codersdk.TemplateVersion, error) {
	f.templateVersionsCount++
	require.Equal(f.t, f.expectedTags, createReq.ProvisionerTags)
	return codersdk.TemplateVersion{
		ID:                  uuid.New(),
		Name:                f.expectedTemplateName,
		MatchedProvisioners: &codersdk.MatchedProvisioners{Count: f.matchedProvisioners},
	}, nil
}

func (f *fakeClient) Upload(ctx context.Context, contentType string, reader io.Reader) (codersdk.UploadResponse, error) {
	f.uploadsCount++
	return codersdk.UploadResponse{
		ID: uuid.New(),
	}, nil
}

func (f *fakeClient) TemplateVersion(ctx context.Context, versionID uuid.UUID) (codersdk.TemplateVersion, error) {
	return codersdk.TemplateVersion{
		ID:                  versionID,
		Job:                 codersdk.ProvisionerJob{Status: f.templateVersionJobStatus},
		MatchedProvisioners: &codersdk.MatchedProvisioners{Count: f.matchedProvisioners},
	}, nil
}
