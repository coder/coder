package db2sdk_test

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

func TestProvisionerJobStatus(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		job    database.ProvisionerJob
		status codersdk.ProvisionerJobStatus
	}{
		{
			name: "canceling",
			job: database.ProvisionerJob{
				CanceledAt: sql.NullTime{
					Time:  dbtime.Now().Add(-time.Minute),
					Valid: true,
				},
			},
			status: codersdk.ProvisionerJobCanceling,
		},
		{
			name: "canceled",
			job: database.ProvisionerJob{
				CanceledAt: sql.NullTime{
					Time:  dbtime.Now().Add(-time.Minute),
					Valid: true,
				},
				CompletedAt: sql.NullTime{
					Time:  dbtime.Now().Add(-30 * time.Second),
					Valid: true,
				},
			},
			status: codersdk.ProvisionerJobCanceled,
		},
		{
			name: "canceled_failed",
			job: database.ProvisionerJob{
				CanceledAt: sql.NullTime{
					Time:  dbtime.Now().Add(-time.Minute),
					Valid: true,
				},
				CompletedAt: sql.NullTime{
					Time:  dbtime.Now().Add(-30 * time.Second),
					Valid: true,
				},
				Error: sql.NullString{String: "badness", Valid: true},
			},
			status: codersdk.ProvisionerJobFailed,
		},
		{
			name:   "pending",
			job:    database.ProvisionerJob{},
			status: codersdk.ProvisionerJobPending,
		},
		{
			name: "succeeded",
			job: database.ProvisionerJob{
				StartedAt: sql.NullTime{
					Time:  dbtime.Now().Add(-time.Minute),
					Valid: true,
				},
				CompletedAt: sql.NullTime{
					Time:  dbtime.Now().Add(-30 * time.Second),
					Valid: true,
				},
			},
			status: codersdk.ProvisionerJobSucceeded,
		},
		{
			name: "completed_failed",
			job: database.ProvisionerJob{
				StartedAt: sql.NullTime{
					Time:  dbtime.Now().Add(-time.Minute),
					Valid: true,
				},
				CompletedAt: sql.NullTime{
					Time:  dbtime.Now().Add(-30 * time.Second),
					Valid: true,
				},
				Error: sql.NullString{String: "badness", Valid: true},
			},
			status: codersdk.ProvisionerJobFailed,
		},
		{
			name: "updated",
			job: database.ProvisionerJob{
				StartedAt: sql.NullTime{
					Time:  dbtime.Now().Add(-time.Minute),
					Valid: true,
				},
				UpdatedAt: dbtime.Now(),
			},
			status: codersdk.ProvisionerJobRunning,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			actual := db2sdk.ProvisionerJobStatus(tc.job)
			require.Equal(t, tc.status, actual)
		})
	}
}

func TestTemplateVersionParameter_OK(t *testing.T) {
	t.Parallel()
	req := require.New(t)

	// In this test we're just going to cover the fields that have to get parsed.
	options := []*proto.RichParameterOption{
		{
			Name:        "foo",
			Description: "bar",
			Value:       "baz",
			Icon:        "David Bowie",
		},
	}
	ob, err := json.Marshal(&options)
	req.NoError(err)

	db := database.TemplateVersionParameter{
		Options:     json.RawMessage(ob),
		Description: "_The Rise and Fall of **Ziggy Stardust** and the Spiders from Mars_",
	}
	sdk, err := db2sdk.TemplateVersionParameter(db)
	req.NoError(err)
	req.Len(sdk.Options, 1)
	req.Equal("foo", sdk.Options[0].Name)
	req.Equal("bar", sdk.Options[0].Description)
	req.Equal("baz", sdk.Options[0].Value)
	req.Equal("David Bowie", sdk.Options[0].Icon)
	req.Equal("The Rise and Fall of Ziggy Stardust and the Spiders from Mars", sdk.DescriptionPlaintext)
}

func TestTemplateVersionParameter_BadOptions(t *testing.T) {
	t.Parallel()
	req := require.New(t)

	db := database.TemplateVersionParameter{
		Options:     json.RawMessage("not really JSON!"),
		Description: "_The Rise and Fall of **Ziggy Stardust** and the Spiders from Mars_",
	}
	_, err := db2sdk.TemplateVersionParameter(db)
	req.Error(err)
}

func TestTemplateVersionParameter_BadDescription(t *testing.T) {
	t.Parallel()
	req := require.New(t)
	desc := make([]byte, 300)
	_, err := rand.Read(desc)
	req.NoError(err)

	db := database.TemplateVersionParameter{
		Options:     json.RawMessage("[]"),
		Description: string(desc),
	}
	sdk, err := db2sdk.TemplateVersionParameter(db)
	// Although the markdown parser can return an error, the way we use it should not, even
	// if we feed it garbage data.
	req.NoError(err)
	req.NotEmpty(sdk.DescriptionPlaintext, "broke the markdown parser with %v", desc)
}
