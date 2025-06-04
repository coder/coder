package cli_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/testutil"
)

var expectedEvents = []cli.ResourceUsageEvent{
	{
		Time:              time.Date(2024, 5, 8, 12, 35, 11, 33732000, time.UTC),
		UserName:          "cian",
		UserID:            uuid.MustParse("17c2bcbc-a768-4e99-a726-6980a9e5524a"),
		TemplateName:      "kubernetes",
		TemplateID:        uuid.MustParse("d9e0f7d1-cc41-4708-ab9e-e4eec271799f"),
		TemplateVersion:   "infallible_swirles1",
		TemplateVersionID: uuid.MustParse("32a67799-18b3-46db-96f3-884f49679066"),
		WorkspaceName:     "harlequin-leech-33",
		WorkspaceID:       uuid.MustParse("a2a16dc3-7a03-49fb-8bfc-f5b9bd5421f9"),
		ResourceID:        "coder/coder-cian-harlequin-leech-33",
		ResourceName:      "main",
		ResourceType:      "kubernetes_deployment",
		ResourceUnit:      "cpu_cores",
		ResourceQuantity:  decimal.New(25, -2),       // 0.25 cores
		DurationSeconds:   decimal.New(65217444, -6), // 65.21744 seconds
		Attributes: map[string]string{
			"namespace": "coder",
		},
	},
	{
		Time:              time.Date(2024, 5, 8, 12, 35, 11, 33732000, time.UTC),
		UserName:          "cian",
		UserID:            uuid.MustParse("17c2bcbc-a768-4e99-a726-6980a9e5524a"),
		TemplateName:      "kubernetes",
		TemplateID:        uuid.MustParse("d9e0f7d1-cc41-4708-ab9e-e4eec271799f"),
		TemplateVersion:   "infallible_swirles1",
		TemplateVersionID: uuid.MustParse("32a67799-18b3-46db-96f3-884f49679066"),
		WorkspaceName:     "harlequin-leech-33",
		WorkspaceID:       uuid.MustParse("a2a16dc3-7a03-49fb-8bfc-f5b9bd5421f9"),
		ResourceID:        "coder/coder-cian-harlequin-leech-33",
		ResourceName:      "main",
		ResourceType:      "kubernetes_deployment",
		ResourceUnit:      "memory_bytes",
		ResourceQuantity:  decimal.New(512*1024*1024, 0), // 512 MiB
		DurationSeconds:   decimal.New(65217444, -6),     // 65.21744 seconds
		Attributes: map[string]string{
			"namespace": "coder",
		},
	},
	{
		Time:              time.Date(2024, 5, 8, 12, 35, 11, 33732000, time.UTC),
		UserName:          "cian",
		UserID:            uuid.MustParse("17c2bcbc-a768-4e99-a726-6980a9e5524a"),
		TemplateName:      "kubernetes",
		TemplateID:        uuid.MustParse("d9e0f7d1-cc41-4708-ab9e-e4eec271799f"),
		TemplateVersion:   "infallible_swirles1",
		TemplateVersionID: uuid.MustParse("32a67799-18b3-46db-96f3-884f49679066"),
		WorkspaceName:     "harlequin-leech-33",
		WorkspaceID:       uuid.MustParse("a2a16dc3-7a03-49fb-8bfc-f5b9bd5421f9"),
		ResourceID:        "coder/coder-cian-harlequin-leech-33-home",
		ResourceName:      "home",
		ResourceType:      "kubernetes_persistent_volume_claim",
		ResourceUnit:      "disk_bytes",
		ResourceQuantity:  decimal.New(1*1024*1024*1024, 1), // 1 GiB
		DurationSeconds:   decimal.New(65217444, -6),        // 65.21744 seconds
		Attributes: map[string]string{
			"namespace":     "coder",
			"storage_class": "",
		},
	},
}

func TestExpResources_TrackUsage(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	log := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	f, err := os.Open("testdata/exp_resources_track_usage.csv.golden")
	require.NoError(t, err)
	defer f.Close()
	wr := cli.WorkspaceBuildInfoCSVReader{R: f}
	builds, err := wr.Read()
	require.NoError(t, err)
	require.Len(t, builds, 2)
	rt := make(cli.ResourceUsageTracker, 0)

	actualEvents := make([]cli.ResourceUsageEvent, 0)
	for _, b := range builds {
		evts, err := rt.Track(ctx, log, b)
		require.NoError(t, err)
		actualEvents = append(actualEvents, evts...)
	}

	assert.Equal(t, len(expectedEvents), len(actualEvents))
	if diff := cmp.Diff(expectedEvents, actualEvents); diff != "" {
		t.Errorf("Track() mismatch (-want +got):\n%s", diff)
	}
}

func TestConvertSIString(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		Input         any
		Expected      decimal.Decimal
		ExpectedError string
	}{
		{
			Input:    "1",
			Expected: decimal.New(1, 0),
		},
		{
			Input:    "3.14159",
			Expected: decimal.New(314159, -5),
		},
		{
			Input:    "1Ki",
			Expected: decimal.New(1024, 0),
		},
		{
			Input:    "1Mi",
			Expected: decimal.New(1024*1024, 0),
		},
		{
			Input:    "1Gi",
			Expected: decimal.New(1024*1024*1024, 0),
		},
		{
			Input:    "1m",
			Expected: decimal.New(1, -3),
		},
	} {
		t.Run(fmt.Sprintf("Input: %v", tc.Input), func(t *testing.T) {
			t.Parallel()
			actual, err := cli.ConvertSIString(tc.Input)
			if tc.ExpectedError != "" {
				require.Error(t, err)
				require.EqualError(t, err, tc.ExpectedError)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.Expected, actual)
			}
		})
	}
}
