package proto_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/proto"
)

func TestLabelsEqual(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		a    []*proto.Stats_Metric_Label
		b    []*proto.Stats_Metric_Label
		eq   bool
	}{
		{
			name: "mainlineEq",
			a: []*proto.Stats_Metric_Label{
				{Name: "credulity", Value: "sus"},
				{Name: "color", Value: "aquamarine"},
			},
			b: []*proto.Stats_Metric_Label{
				{Name: "credulity", Value: "sus"},
				{Name: "color", Value: "aquamarine"},
			},
			eq: true,
		},
		{
			name: "emptyValue",
			a: []*proto.Stats_Metric_Label{
				{Name: "credulity", Value: "sus"},
				{Name: "color", Value: "aquamarine"},
				{Name: "singularity", Value: ""},
			},
			b: []*proto.Stats_Metric_Label{
				{Name: "credulity", Value: "sus"},
				{Name: "color", Value: "aquamarine"},
			},
			eq: true,
		},
		{
			name: "extra",
			a: []*proto.Stats_Metric_Label{
				{Name: "credulity", Value: "sus"},
				{Name: "color", Value: "aquamarine"},
				{Name: "opacity", Value: "seyshells"},
			},
			b: []*proto.Stats_Metric_Label{
				{Name: "credulity", Value: "sus"},
				{Name: "color", Value: "aquamarine"},
			},
			eq: false,
		},
		{
			name: "different",
			a: []*proto.Stats_Metric_Label{
				{Name: "credulity", Value: "sus"},
				{Name: "color", Value: "aquamarine"},
			},
			b: []*proto.Stats_Metric_Label{
				{Name: "credulity", Value: "legit"},
				{Name: "color", Value: "aquamarine"},
			},
			eq: false,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.eq, proto.LabelsEqual(tc.a, tc.b))
			require.Equal(t, tc.eq, proto.LabelsEqual(tc.b, tc.a))
		})
	}
}
