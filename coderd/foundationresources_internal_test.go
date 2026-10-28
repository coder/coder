package coderd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestFoundationResources(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		state     string
		want      []codersdk.FoundationResource
		available bool
	}{
		{name: "NoState", available: true},
		{name: "EmptyState", state: `{"version":4,"resources":[]}`, available: true},
		{name: "Malformed", state: `{"version":4,"resources":`},
		{name: "UnsupportedVersion", state: `{"version":3,"resources":[]}`},
		{name: "NullState", state: `null`},
		{name: "InvalidResource", state: `{"version":4,"resources":[{"mode":"managed","instances":[{}]}]}`},
		{name: "InvalidInstance", state: `{"version":4,"resources":[{"mode":"managed","type":"x","name":"y","instances":[null]}]}`},
		{
			name: "ManagedInventory",
			state: `{"version":4,"outputs":{"secret":{"value":"do-not-expose","sensitive":true}},"resources":[
				{"mode":"managed","type":"terraform_data","name":"root","instances":[{"attributes":{"id":"private-id","input":"do-not-expose"},"private":"private-data"}]},
				{"module":"module.cluster[\"east\"].module.network","mode":"managed","type":"aws_subnet","name":"private","instances":[{"index_key":0},{"index_key":1}]},
				{"module":"module.cluster[\"west\"].module.network","mode":"managed","type":"aws_subnet","name":"private","instances":[{"index_key":"a"}]},
				{"mode":"data","type":"aws_region","name":"current","instances":[{}]},
				{"mode":"managed","type":"aws_instance","name":"zero","instances":[]}
			]}`,
			want: []codersdk.FoundationResource{
				{Address: `module.cluster["east"].module.network.aws_subnet.private`, Type: "aws_subnet", Name: "private", Instances: 2},
				{Address: `module.cluster["west"].module.network.aws_subnet.private`, Type: "aws_subnet", Name: "private", Instances: 1},
				{Address: "terraform_data.root", Type: "terraform_data", Name: "root", Instances: 1},
			},
			available: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, available := foundationResources([]byte(tc.state))
			require.Equal(t, tc.available, available)
			if tc.want == nil {
				require.Equal(t, []codersdk.FoundationResource{}, got)
			} else {
				require.Equal(t, tc.want, got)
			}
		})
	}
}
