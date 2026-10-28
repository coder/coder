package codersdk_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func TestJobReaperMaxPendingAge(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		args      []string
		environ   serpent.Environ
		config    string
		want      time.Duration
		wantError bool
	}{
		{name: "Default", want: 2 * time.Hour},
		{name: "Flag", args: []string{"--job-reaper-max-pending-age", "1h"}, want: time.Hour},
		{name: "Environment", environ: serpent.Environ{{Name: "CODER_JOB_REAPER_MAX_PENDING_AGE", Value: "3h"}}, want: 3 * time.Hour},
		{name: "YAML", config: "jobReaperMaxPendingAge: 4h\n", want: 4 * time.Hour},
		{name: "Zero", args: []string{"--job-reaper-max-pending-age", "0"}, wantError: true},
		{name: "Negative", args: []string{"--job-reaper-max-pending-age=-1h"}, wantError: true},
		{name: "BelowOrdinaryTimeout", args: []string{"--job-reaper-max-pending-age", "10m"}, wantError: true},
		{name: "EqualToOrdinaryTimeout", args: []string{"--job-reaper-max-pending-age", "30m"}, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			values := &codersdk.DeploymentValues{}
			called := false
			cmd := &serpent.Command{
				Use:     "test",
				Options: values.Options(),
				Handler: func(_ *serpent.Invocation) error {
					called = true
					require.Equal(t, tc.want, values.JobReaperMaxPendingAge.Value())
					return nil
				},
			}
			inv := cmd.Invoke(tc.args...)
			inv.Environ = append(serpent.Environ{}, tc.environ...)
			if tc.config != "" {
				path := filepath.Join(t.TempDir(), "config.yaml")
				require.NoError(t, os.WriteFile(path, []byte(tc.config), 0o600))
				inv.Environ = append(inv.Environ, serpent.EnvVar{Name: "CODER_CONFIG_PATH", Value: path})
			}
			err := inv.Run()
			if tc.wantError {
				require.ErrorContains(t, err, "must be greater than 30 minutes")
				require.False(t, called)
			} else {
				require.NoError(t, err)
				require.True(t, called)
			}
		})
	}
}
