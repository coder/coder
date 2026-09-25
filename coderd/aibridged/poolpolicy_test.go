package aibridged_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func TestPoolOptionsFromConfig(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name                        string
		cfg                         codersdk.AIBridgeConfig
		wantStructuredLogging       bool
		wantDisableContentRecording bool
		wantWarnContentNotExported  bool
	}{
		{
			name: "Defaults",
		},
		{
			name: "StructuredLoggingWithoutSource",
			cfg:  codersdk.AIBridgeConfig{StructuredLogging: serpent.Bool(true)},
		},
		{
			name: "SourceCoderd",
			cfg: codersdk.AIBridgeConfig{
				StructuredLogging:       serpent.Bool(true),
				StructuredLoggingSource: string(codersdk.AIStructuredLoggingSourceCoderd),
			},
		},
		{
			name: "SourceGateway",
			cfg: codersdk.AIBridgeConfig{
				StructuredLogging:       serpent.Bool(true),
				StructuredLoggingSource: string(codersdk.AIStructuredLoggingSourceGateway),
			},
			wantStructuredLogging: true,
		},
		{
			name: "SourceBoth",
			cfg: codersdk.AIBridgeConfig{
				StructuredLogging:       serpent.Bool(true),
				StructuredLoggingSource: string(codersdk.AIStructuredLoggingSourceBoth),
			},
			wantStructuredLogging: true,
		},
		{
			name: "SourceGatewayWithoutStructuredLogging",
			cfg: codersdk.AIBridgeConfig{
				StructuredLoggingSource: string(codersdk.AIStructuredLoggingSourceGateway),
			},
		},
		{
			// Nothing else reports that the dropped records are exported
			// nowhere, so the derivation has to.
			name:                        "ContentRecordingDisabledWithoutGatewayLogs",
			cfg:                         codersdk.AIBridgeConfig{DisableContentRecording: serpent.Bool(true)},
			wantDisableContentRecording: true,
			wantWarnContentNotExported:  true,
		},
		{
			name: "ContentRecordingDisabledWithGatewayLogs",
			cfg: codersdk.AIBridgeConfig{
				DisableContentRecording: serpent.Bool(true),
				StructuredLogging:       serpent.Bool(true),
				StructuredLoggingSource: string(codersdk.AIStructuredLoggingSourceGateway),
			},
			wantStructuredLogging:       true,
			wantDisableContentRecording: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			options := aibridged.PoolOptionsFromConfig(tc.cfg)

			require.Equal(t, tc.wantStructuredLogging, options.StructuredLogging, "StructuredLogging")
			require.Equal(t, tc.wantDisableContentRecording, options.DisableContentRecording, "DisableContentRecording")
			require.Equal(t, tc.wantWarnContentNotExported, options.WarnContentNotExported, "WarnContentNotExported")

			// The record policy must not disturb the cache sizing the
			// deployment relies on.
			require.Equal(t, aibridged.DefaultPoolOptions.MaxItems, options.MaxItems, "MaxItems")
			require.Equal(t, aibridged.DefaultPoolOptions.TTL, options.TTL, "TTL")
		})
	}
}
