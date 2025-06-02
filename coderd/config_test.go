package coderd_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func TestProcessRoleTokenLifetimesConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                    string
		roleTokenLifetimesJSON  string
		expectedParsedLifetimes map[string]time.Duration
		expectError             bool
	}{
		{
			name:                    "EmptyJSON",
			roleTokenLifetimesJSON:  "",
			expectedParsedLifetimes: map[string]time.Duration{},
			expectError:             false,
		},
		{
			name:                    "EmptyJSONObject",
			roleTokenLifetimesJSON:  "{}",
			expectedParsedLifetimes: map[string]time.Duration{},
			expectError:             false,
		},
		{
			name:                   "ValidJSON_SiteWideRolesOnly",
			roleTokenLifetimesJSON: `{"owner": "168h", "user-admin": "72h", "member": "24h"}`,
			expectedParsedLifetimes: map[string]time.Duration{
				"owner":      168 * time.Hour,
				"user-admin": 72 * time.Hour,
				"member":     24 * time.Hour,
			},
			expectError: false,
		},
		{
			name:                    "MalformedJSON",
			roleTokenLifetimesJSON:  `{"owner": "168h"`,
			expectedParsedLifetimes: map[string]time.Duration{},
			expectError:             true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Setup using coderdtest
			client, db := coderdtest.NewWithDatabase(t, nil)
			_ = coderdtest.CreateFirstUser(t, client)

			logger := slog.Make(sloghuman.Sink(io.Discard))
			ctx := context.Background()

			// Create deployment values with the test JSON
			sl := codersdk.SessionLifetime{
				RoleTokenLifetimes: serpent.String(tt.roleTokenLifetimesJSON),
			}

			// Execute
			err := coderd.ProcessRoleTokenLifetimesConfig(ctx, &sl, db, logger)

			// Verify
			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Verify parsed lifetimes by testing the behavior through GetMaxTokenLifetimeForRole
			for expectedKey, expectedDuration := range tt.expectedParsedLifetimes {
				role, err := rbac.RoleNameFromString(expectedKey)
				require.NoError(t, err)
				actualDuration := sl.MaxTokenLifetimeForRole(role)
				assert.Equal(t, expectedDuration, actualDuration, "Duration mismatch for role %s", expectedKey)
			}
		})
	}
}
