package codersdk_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestAIBridgeAttributionMarshalJSON(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name        string
		attribution codersdk.AIBridgeAttribution
		want        string
	}{
		{name: "nil", want: `{}`},
		{name: "empty", attribution: codersdk.AIBridgeAttribution{}, want: `{}`},
		{name: "known", attribution: codersdk.AIBridgeAttribution{"workspace_id": "workspace-id"}, want: `{"workspace_id":"workspace-id"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			data, err := json.Marshal(tt.attribution)
			require.NoError(t, err)
			require.JSONEq(t, tt.want, string(data))

			for _, value := range []any{
				codersdk.AIBridgeThread{Attribution: tt.attribution},
				codersdk.AIBridgeAgenticAction{Attribution: tt.attribution},
			} {
				data, err := json.Marshal(value)
				require.NoError(t, err)
				var fields map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(data, &fields))
				require.JSONEq(t, tt.want, string(fields["attribution"]))
			}
		})
	}
}
