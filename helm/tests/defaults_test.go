package tests // nolint: testpackage

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultValues(t *testing.T) {
	t.Parallel()

	t.Run("MissingRequiredValuesFailsToRender", func(t *testing.T) {
		t.Parallel()

		// Given: we load the chart successfully
		chart, err := LoadChart()
		require.NoError(t, err, "failed to load chart")
		require.NoError(t, chart.Validate(), "chart validation failed")

		// Ensure that the correct metadata is set
		require.Equal(t, "coder", chart.Metadata.Name, "chart name is incorrect")
		require.False(t, chart.Metadata.Deprecated, "chart should not be deprecated")

		// When: the user does not set the coder.image.tag value
		_, err = chart.Render(func(v *Values) {
			v.Coder.Image.Tag = ""
		}, nil, nil)

		// Then: The chart should fail to render and the user should be informed
		// that they must set the coder.image.tag value.
		require.Error(t, err)
		require.Contains(t, err.Error(), "You must specify the coder.image.tag value if you're installing the Helm chart directly from Git.")
	})

	t.Run("RendersSuccessfullyWithDefaultValues", func(t *testing.T) {
		t.Parallel()

		// Given: we load the chart successfully
		chart, err := LoadChart()
		require.NoError(t, err, "failed to load chart")
		require.NoError(t, chart.Validate(), "chart validation failed")

		// When: the user sets the coder.image.tag to a valid value
		objs, err := chart.Render(func(v *Values) {
			v.Coder.Image.Tag = "latest"
		}, nil, nil)

		// Then: The chart should render successfully
		require.NoError(t, err, "failed to render manifests")

		// And: The deployment should have the correct default values
		deployment := requireDeployment(t, objs, "coder")
		require.Equal(t, int32(1), *deployment.Spec.Replicas, "expected 1 replica")
		require.Len(t, deployment.Spec.Template.Spec.Containers, 1, "expected 1 container")
		require.Equal(t, "coder", deployment.Spec.Template.Spec.Containers[0].Name, "unexpected container name")
		require.Equal(t, "ghcr.io/coder/coder:latest", deployment.Spec.Template.Spec.Containers[0].Image, "unexpected image")
		require.Equal(t, "IfNotPresent", string(deployment.Spec.Template.Spec.Containers[0].ImagePullPolicy), "unexpected image pull policy")
		require.Empty(t, deployment.Spec.Template.Spec.InitContainers, "expected no init containers")
		require.Empty(t, deployment.Spec.Template.Annotations, "expected no annotations")
		// Ensure default env vars are set
		requireEnv(t, deployment.Spec.Template.Spec.Containers[0].Env, "CODER_HTTP_ADDRESS", "0.0.0.0:8080")
	})

	t.Run("TLS", func(t *testing.T) {
		t.Parallel()

		// Given: we load the chart successfully
		chart, err := LoadChart()
		require.NoError(t, err, "failed to load chart")
		require.NoError(t, chart.Validate(), "chart validation failed")

		// When: the user sets coder.tls.secretNames to a valid value
		objs, err := chart.Render(func(v *Values) {
			v.Coder.Image.Tag = "latest"
			v.Coder.TLS.SecretNames = []string{"coder-tls"}
		}, nil, nil)

		// Then: The chart should render successfully
		require.NoError(t, err, "failed to render manifests")

		// And: the CODER_TLS_ADDRESS env var should be set
		deployment := requireDeployment(t, objs, "coder")
		require.Equal(t, int32(1), *deployment.Spec.Replicas, "expected 1 replica")
		require.Len(t, deployment.Spec.Template.Spec.Containers, 1, "expected 1 container")
		requireEnv(t, deployment.Spec.Template.Spec.Containers[0].Env, "CODER_TLS_ADDRESS", "0.0.0.0:8443")
	})
}
