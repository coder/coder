package coderd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// TestClaudePlatformEnvChangesAreDrift pins every operator-controllable Claude
// Platform for AWS env field into the canonical provider hash, so changing any
// of them in the environment is reported as drift on restart instead of being
// silently ignored.
func TestClaudePlatformEnvChangesAreDrift(t *testing.T) {
	t.Parallel()

	baseline := codersdk.AIProviderConfig{
		Type:                          string(database.AIProviderTypeAnthropic),
		Name:                          "claude-platform-drift",
		ClaudePlatformAuthMode:        string(codersdk.AIProviderClaudePlatformAWSAuthModeIAM),
		ClaudePlatformRegion:          "us-east-1",
		ClaudePlatformWorkspaceID:     "ws-drift",
		ClaudePlatformAccessKey:       "AKID-cp",
		ClaudePlatformAccessKeySecret: "cp-secret",
		ClaudePlatformRoleARN:         "arn:aws:iam::123456789012:role/ClaudePlatform",
	}

	tests := []struct {
		name   string
		mutate func(*codersdk.AIProviderConfig)
	}{
		{
			name: "AuthMode",
			mutate: func(p *codersdk.AIProviderConfig) {
				p.ClaudePlatformAuthMode = string(codersdk.AIProviderClaudePlatformAWSAuthModeAPIKey)
			},
		},
		{
			name: "Region",
			mutate: func(p *codersdk.AIProviderConfig) {
				p.ClaudePlatformRegion = "eu-central-1"
			},
		},
		{
			name: "WorkspaceID",
			mutate: func(p *codersdk.AIProviderConfig) {
				p.ClaudePlatformWorkspaceID = "ws-drift-rotated"
			},
		},
		{
			name: "RoleARN",
			mutate: func(p *codersdk.AIProviderConfig) {
				p.ClaudePlatformRoleARN = "arn:aws:iam::123456789012:role/ClaudePlatformOther"
			},
		},
		{
			name: "AccessKey",
			mutate: func(p *codersdk.AIProviderConfig) {
				p.ClaudePlatformAccessKey = "AKID-cp-rotated"
			},
		},
		{
			name: "AccessKeySecret",
			mutate: func(p *codersdk.AIProviderConfig) {
				p.ClaudePlatformAccessKeySecret = "cp-secret-rotated"
			},
		},
	}

	baselineHash := claudePlatformEnvHash(t, baseline)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mutated := baseline
			tt.mutate(&mutated)
			assert.NotEqual(t, baselineHash, claudePlatformEnvHash(t, mutated))
		})
	}

	t.Run("EveryFieldHashesDistinctly", func(t *testing.T) {
		t.Parallel()
		// Distinct per field, not merely distinct from the baseline: a hash
		// that collapsed two fields would still differ from the baseline but
		// would mask a change from one value to the other.
		byHash := map[string]string{baselineHash: "baseline"}
		for _, tt := range tests {
			mutated := baseline
			tt.mutate(&mutated)
			hash := claudePlatformEnvHash(t, mutated)
			other, exists := byHash[hash]
			require.False(t, exists, "%s and %s hash identically", tt.name, other)
			byHash[hash] = tt.name
		}
	})
}

// claudePlatformEnvHash runs a single indexed provider config through the
// env-to-desired-provider normalization and returns its canonical hash.
func claudePlatformEnvHash(t *testing.T, p codersdk.AIProviderConfig) string {
	t.Helper()
	desired, err := providersFromEnv(t.Context(), codersdk.AIBridgeConfig{
		Providers: []codersdk.AIProviderConfig{p},
	}, slogtest.Make(t, nil))
	require.NoError(t, err)
	require.Len(t, desired, 1)
	require.NotNil(t, desired[0].ClaudePlatformAWS, "config must normalize into Claude Platform settings")
	require.NotEmpty(t, desired[0].Hash)
	return desired[0].Hash
}
