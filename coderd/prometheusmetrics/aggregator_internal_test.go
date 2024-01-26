package prometheusmetrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/proto"
)

func TestAnnotatedMetric_Is(t *testing.T) {
	t.Parallel()
	am1 := &annotatedMetric{
		Stats_Metric: &proto.Stats_Metric{
			Name:  "met",
			Type:  proto.Stats_Metric_COUNTER,
			Value: 1,
			Labels: []*proto.Stats_Metric_Label{
				{Name: "rarity", Value: "blue moon"},
				{Name: "certainty", Value: "yes"},
			},
		},
		username:      "spike",
		workspaceName: "work",
		agentName:     "janus",
		templateName:  "tempe",
		expiryDate:    time.Now(),
	}
	for _, tc := range []struct {
		name string
		req  updateRequest
		m    *proto.Stats_Metric
		is   bool
	}{
		{
			name: "OK",
			req: updateRequest{
				username:      "spike",
				workspaceName: "work",
				agentName:     "janus",
				templateName:  "tempe",
				metrics:       nil,
				timestamp:     time.Now().Add(-5 * time.Second),
			},
			m: &proto.Stats_Metric{
				Name:  "met",
				Type:  proto.Stats_Metric_COUNTER,
				Value: 2,
				Labels: []*proto.Stats_Metric_Label{
					{Name: "rarity", Value: "blue moon"},
					{Name: "certainty", Value: "yes"},
				},
			},
			is: true,
		},
		{
			name: "missingLabel",
			req: updateRequest{
				username:      "spike",
				workspaceName: "work",
				agentName:     "janus",
				templateName:  "tempe",
				metrics:       nil,
				timestamp:     time.Now().Add(-5 * time.Second),
			},
			m: &proto.Stats_Metric{
				Name:  "met",
				Type:  proto.Stats_Metric_COUNTER,
				Value: 2,
				Labels: []*proto.Stats_Metric_Label{
					{Name: "certainty", Value: "yes"},
				},
			},
			is: false,
		},
		{
			name: "wrongLabelValue",
			req: updateRequest{
				username:      "spike",
				workspaceName: "work",
				agentName:     "janus",
				templateName:  "tempe",
				metrics:       nil,
				timestamp:     time.Now().Add(-5 * time.Second),
			},
			m: &proto.Stats_Metric{
				Name:  "met",
				Type:  proto.Stats_Metric_COUNTER,
				Value: 2,
				Labels: []*proto.Stats_Metric_Label{
					{Name: "rarity", Value: "blue moon"},
					{Name: "certainty", Value: "inshallah"},
				},
			},
			is: false,
		},
		{
			name: "wrongMetricName",
			req: updateRequest{
				username:      "spike",
				workspaceName: "work",
				agentName:     "janus",
				templateName:  "tempe",
				metrics:       nil,
				timestamp:     time.Now().Add(-5 * time.Second),
			},
			m: &proto.Stats_Metric{
				Name:  "cub",
				Type:  proto.Stats_Metric_COUNTER,
				Value: 2,
				Labels: []*proto.Stats_Metric_Label{
					{Name: "rarity", Value: "blue moon"},
					{Name: "certainty", Value: "yes"},
				},
			},
			is: false,
		},
		{
			name: "wrongUsername",
			req: updateRequest{
				username:      "steve",
				workspaceName: "work",
				agentName:     "janus",
				templateName:  "tempe",
				metrics:       nil,
				timestamp:     time.Now().Add(-5 * time.Second),
			},
			m: &proto.Stats_Metric{
				Name:  "met",
				Type:  proto.Stats_Metric_COUNTER,
				Value: 2,
				Labels: []*proto.Stats_Metric_Label{
					{Name: "rarity", Value: "blue moon"},
					{Name: "certainty", Value: "yes"},
				},
			},
			is: false,
		},
		{
			name: "wrongWorkspaceName",
			req: updateRequest{
				username:      "spike",
				workspaceName: "play",
				agentName:     "janus",
				templateName:  "tempe",
				metrics:       nil,
				timestamp:     time.Now().Add(-5 * time.Second),
			},
			m: &proto.Stats_Metric{
				Name:  "met",
				Type:  proto.Stats_Metric_COUNTER,
				Value: 2,
				Labels: []*proto.Stats_Metric_Label{
					{Name: "rarity", Value: "blue moon"},
					{Name: "certainty", Value: "yes"},
				},
			},
			is: false,
		},
		{
			name: "wrongAgentName",
			req: updateRequest{
				username:      "spike",
				workspaceName: "work",
				agentName:     "bond",
				templateName:  "tempe",
				metrics:       nil,
				timestamp:     time.Now().Add(-5 * time.Second),
			},
			m: &proto.Stats_Metric{
				Name:  "met",
				Type:  proto.Stats_Metric_COUNTER,
				Value: 2,
				Labels: []*proto.Stats_Metric_Label{
					{Name: "rarity", Value: "blue moon"},
					{Name: "certainty", Value: "yes"},
				},
			},
			is: false,
		},
		{
			name: "wrongTemplateName",
			req: updateRequest{
				username:      "spike",
				workspaceName: "work",
				agentName:     "janus",
				templateName:  "phoenix",
				metrics:       nil,
				timestamp:     time.Now().Add(-5 * time.Second),
			},
			m: &proto.Stats_Metric{
				Name:  "met",
				Type:  proto.Stats_Metric_COUNTER,
				Value: 2,
				Labels: []*proto.Stats_Metric_Label{
					{Name: "rarity", Value: "blue moon"},
					{Name: "certainty", Value: "yes"},
				},
			},
			is: false,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.is, am1.is(tc.req, tc.m))
		})
	}
}
