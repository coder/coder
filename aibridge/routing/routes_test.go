package routing_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/routing"
)

func TestMetricRoute(t *testing.T) {
	t.Parallel()

	require.Equal(t, "/", routing.MetricRoute(""))
	require.Equal(t, "/v1/models", routing.MetricRoute("/v1/models"))
}

func TestMetricMethod(t *testing.T) {
	t.Parallel()

	require.Equal(t, http.MethodPost, routing.MetricMethod(http.MethodPost))
	require.Equal(t, "OTHER", routing.MetricMethod("CUSTOM"))
}
