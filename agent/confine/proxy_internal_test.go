package confine

import (
	"testing"

	"github.com/stretchr/testify/require"

	sandboxpolicy "github.com/coder/coder/coder-sandbox/policy"
	sandboxproxy "github.com/coder/coder/coder-sandbox/proxy"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

func TestSandboxEventRecorderCollapsesPolicyPhases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		events []sandboxproxy.Event
		want   NetworkEvent
	}{
		{
			name: "public resolution completes allow",
			events: []sandboxproxy.Event{
				{Session: 1, Kind: sandboxproxy.KindConnect, Action: sandboxpolicy.ActionAllow, Host: "Example.COM.", Port: "443", Generation: 3},
				{Session: 1, Kind: sandboxproxy.KindResolution, Proto: sandboxproxy.ProtoHTTPS, Host: "example.com", Port: "443", ResolvedIP: "203.0.113.10", Generation: 3},
				{Session: 1, Kind: sandboxproxy.KindResponse, Status: 200},
			},
			want: NetworkEvent{
				Protocol: agentsdk.AISandboxNetworkProtocolConnect, Host: "example.com", Port: 443,
				Action: agentsdk.AISandboxNetworkEventActionAllowed, PolicyRevision: 3,
			},
		},
		{
			name: "resolved IP decision replaces name allow",
			events: []sandboxproxy.Event{
				{Session: 2, Kind: sandboxproxy.KindRequest, Proto: sandboxproxy.ProtoHTTP, Method: "GET", Action: sandboxpolicy.ActionAllow, Host: "private.example", Port: "80", Generation: 4},
				{Session: 2, Kind: sandboxproxy.KindResolution, Proto: sandboxproxy.ProtoHTTP, Method: "GET", Host: "private.example", Port: "80", ResolvedIP: "10.0.0.1", Generation: 4},
				{Session: 2, Kind: sandboxproxy.KindRequest, Proto: sandboxproxy.ProtoHTTP, Method: "GET", Phase: "ip", Action: sandboxpolicy.ActionDeny, Host: "private.example", Port: "80", Generation: 4},
			},
			want: NetworkEvent{
				Protocol: agentsdk.AISandboxNetworkProtocolHTTP, Host: "private.example", Port: 80,
				Action: agentsdk.AISandboxNetworkEventActionDenied, PolicyRevision: 4,
			},
		},
		{
			name: "name denial is terminal",
			events: []sandboxproxy.Event{
				{Session: 3, Kind: sandboxproxy.KindConnect, Action: sandboxpolicy.ActionDeny, Host: "denied.example", Port: "443", Generation: 5},
			},
			want: NetworkEvent{
				Protocol: agentsdk.AISandboxNetworkProtocolConnect, Host: "denied.example", Port: 443,
				Action: agentsdk.AISandboxNetworkEventActionDenied, PolicyRevision: 5,
			},
		},
		{
			name: "literal private exemption completes at resolution",
			events: []sandboxproxy.Event{
				{Session: 4, Kind: sandboxproxy.KindConnect, Action: sandboxpolicy.ActionAllow, Host: "127.0.0.1", Port: "443", Generation: 6},
				{Session: 4, Kind: sandboxproxy.KindResolution, Proto: sandboxproxy.ProtoHTTPS, Host: "127.0.0.1", Port: "443", ResolvedIP: "127.0.0.1", Generation: 6},
			},
			want: NetworkEvent{
				Protocol: agentsdk.AISandboxNetworkProtocolConnect, Host: "127.0.0.1", Port: 443,
				Action: agentsdk.AISandboxNetworkEventActionAllowed, PolicyRevision: 6,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got []NetworkEvent
			recorder := newSandboxEventRecorder(func(event NetworkEvent) {
				got = append(got, event)
			})
			for _, event := range tt.events {
				recorder.Record(event)
			}
			require.Equal(t, []NetworkEvent{tt.want}, got)
		})
	}
}
