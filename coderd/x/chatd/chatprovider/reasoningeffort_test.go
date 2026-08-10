package chatprovider_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
)

func TestResolveReasoningEffort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		requested *string
		config    *codersdk.ChatModelReasoningEffortConfig
		want      *string
	}{
		{name: "NilConfigIgnoresRequested", requested: new(codersdk.ChatModelReasoningEffortHigh)},
		{name: "DefaultUsedWhenNoRequested", config: effortConfig("medium", "high"), want: new(codersdk.ChatModelReasoningEffortMedium)},
		{name: "RequestedWinsOverDefault", requested: new(codersdk.ChatModelReasoningEffortHigh), config: effortConfig("medium", "high"), want: new(codersdk.ChatModelReasoningEffortHigh)},
		{name: "RequestedWinsWithoutMax", requested: new(codersdk.ChatModelReasoningEffortHigh), config: effortConfig("medium", ""), want: new(codersdk.ChatModelReasoningEffortHigh)},
		{name: "RequestedClampedToMax", requested: new(codersdk.ChatModelReasoningEffortXHigh), config: effortConfig("low", "medium"), want: new(codersdk.ChatModelReasoningEffortMedium)},
		{name: "DefaultClampedToMax", config: effortConfig("xhigh", "medium"), want: new(codersdk.ChatModelReasoningEffortMedium)},
		{name: "InvalidRequestedFallsBackToDefault", requested: ptr.Ref(" HIGH "), config: effortConfig("low", "high"), want: new(codersdk.ChatModelReasoningEffortLow)},
		{name: "InvalidMaxReturnsNil", requested: new(codersdk.ChatModelReasoningEffortMedium), config: effortConfig("low", " HIGH ")},
		{name: "EmptyConfigReturnsNil", config: &codersdk.ChatModelReasoningEffortConfig{}},
		{name: "MaxSupported", requested: new(codersdk.ChatModelReasoningEffortMax), config: effortConfig("medium", "max"), want: new(codersdk.ChatModelReasoningEffortMax)},
		{name: "NoneSupported", requested: new(codersdk.ChatModelReasoningEffortNone), config: effortConfig("medium", "xhigh"), want: new(codersdk.ChatModelReasoningEffortNone)},
		{name: "MaxOnlyConfigClampsRequested", requested: new(codersdk.ChatModelReasoningEffortXHigh), config: effortConfig("", "medium"), want: new(codersdk.ChatModelReasoningEffortMedium)},
		{name: "MaxOnlyConfigWithoutRequestedReturnsNil", config: effortConfig("", "medium")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := chatprovider.ResolveReasoningEffort(tt.requested, tt.config)
			if tt.want == nil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, *tt.want, *got)
		})
	}
}

func TestSelectableReasoningEfforts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config *codersdk.ChatModelReasoningEffortConfig
		want   []string
	}{
		{name: "NilConfig"},
		{name: "NoMax", config: effortConfig("medium", "")},
		{name: "UnknownMax", config: effortConfig("medium", " HIGH ")},
		{name: "ThroughMedium", config: effortConfig("low", "medium"), want: []string{"none", "minimal", "low", "medium"}},
		{name: "ThroughMax", config: effortConfig("medium", "max"), want: []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, chatprovider.SelectableReasoningEfforts(tt.config))
		})
	}
}

func effortConfig(defaultEffort, maxEffort string) *codersdk.ChatModelReasoningEffortConfig {
	cfg := &codersdk.ChatModelReasoningEffortConfig{}
	if defaultEffort != "" {
		cfg.Default = ptr.Ref(defaultEffort)
	}
	if maxEffort != "" {
		cfg.Max = ptr.Ref(maxEffort)
	}
	return cfg
}
