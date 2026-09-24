package provider

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/config"
)

func TestBedrockInvocationModelUsesConfiguredRewriteTarget(t *testing.T) {
	t.Parallel()

	p := newTestBedrock(t, config.Anthropic{BaseURL: "http://bedrock.example"}, config.AWSBedrock{
		Region:                 "us-west-2",
		AccessKey:              "test-key",
		AccessKeySecret:        "test-secret",
		Model:                  "arn:aws:bedrock:us-west-2:123456789012:inference-profile/primary",
		SmallFastModel:         "arn:aws:bedrock:us-west-2:123456789012:inference-profile/fast",
		ResolvedModel:          "anthropic.claude-opus-4-5",
		ResolvedSmallFastModel: "anthropic.claude-haiku-4-5",
	})

	req := httptest.NewRequest(http.MethodPost, routeMessages, bytes.NewBufferString(`{"model":"claude-haiku-4-5","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`))
	interceptor, err := p.CreateInterceptor(httptest.NewRecorder(), req, testTracer)
	require.NoError(t, err)
	require.Equal(t, "anthropic.claude-haiku-4-5", interceptor.Model())
	require.Equal(t, "arn:aws:bedrock:us-west-2:123456789012:inference-profile/fast", interceptor.InvocationModel())
}
