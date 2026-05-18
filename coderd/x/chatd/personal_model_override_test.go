package chatd_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
)

func TestChatPersonalModelOverrideKey(t *testing.T) {
	t.Parallel()

	require.Equal(
		t,
		"chat_personal_model_override:root",
		chatd.ChatPersonalModelOverrideKey(codersdk.ChatPersonalModelOverrideContextRoot),
	)
}

func TestParseChatPersonalModelOverride(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	tests := []struct {
		name        string
		raw         string
		defaultMode codersdk.ChatPersonalModelOverrideMode
		want        chatd.ParsedChatPersonalModelOverride
	}{
		{
			name:        "EmptyUsesDefault",
			raw:         "",
			defaultMode: codersdk.ChatPersonalModelOverrideModeDeploymentDefault,
			want: chatd.ParsedChatPersonalModelOverride{
				Mode: codersdk.ChatPersonalModelOverrideModeDeploymentDefault,
			},
		},
		{
			name:        "ChatDefault",
			raw:         string(codersdk.ChatPersonalModelOverrideModeChatDefault),
			defaultMode: codersdk.ChatPersonalModelOverrideModeDeploymentDefault,
			want: chatd.ParsedChatPersonalModelOverride{
				Mode: codersdk.ChatPersonalModelOverrideModeChatDefault,
			},
		},
		{
			name:        "DeploymentDefault",
			raw:         string(codersdk.ChatPersonalModelOverrideModeDeploymentDefault),
			defaultMode: codersdk.ChatPersonalModelOverrideModeChatDefault,
			want: chatd.ParsedChatPersonalModelOverride{
				Mode: codersdk.ChatPersonalModelOverrideModeDeploymentDefault,
			},
		},
		{
			name:        "Model",
			raw:         "model:" + modelConfigID.String(),
			defaultMode: codersdk.ChatPersonalModelOverrideModeDeploymentDefault,
			want: chatd.ParsedChatPersonalModelOverride{
				Mode:          codersdk.ChatPersonalModelOverrideModeModel,
				ModelConfigID: modelConfigID,
			},
		},
		{
			name:        "InvalidModelUUID",
			raw:         "model:not-a-uuid",
			defaultMode: codersdk.ChatPersonalModelOverrideModeDeploymentDefault,
			want: chatd.ParsedChatPersonalModelOverride{
				Mode:      codersdk.ChatPersonalModelOverrideModeDeploymentDefault,
				Malformed: true,
			},
		},
		{
			name:        "UnknownValue",
			raw:         "unknown",
			defaultMode: codersdk.ChatPersonalModelOverrideModeChatDefault,
			want: chatd.ParsedChatPersonalModelOverride{
				Mode:      codersdk.ChatPersonalModelOverrideModeChatDefault,
				Malformed: true,
			},
		},
		{
			name:        "OuterWhitespace",
			raw:         " \tmodel:" + modelConfigID.String() + "\n",
			defaultMode: codersdk.ChatPersonalModelOverrideModeDeploymentDefault,
			want: chatd.ParsedChatPersonalModelOverride{
				Mode:          codersdk.ChatPersonalModelOverrideModeModel,
				ModelConfigID: modelConfigID,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := chatd.ParseChatPersonalModelOverride(tt.raw, tt.defaultMode)
			require.Equal(t, tt.want, got)
		})
	}
}
