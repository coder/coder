package provider

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
)

func claudePlatformIAMCfg() *config.AWSClaudePlatform {
	return &config.AWSClaudePlatform{Region: "us-west-2", WorkspaceID: "wrkspc_config"}
}

func newTestClaudePlatform(t testing.TB, cfg config.Anthropic, cpCfg *config.AWSClaudePlatform) *Anthropic {
	t.Helper()
	p, err := NewAnthropic(context.Background(), cfg, nil, cpCfg)
	require.NoError(t, err)
	p.claudePlatform.creds = aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
		return aws.Credentials{AccessKeyID: "test-access-key", SecretAccessKey: "test-secret-key", SessionToken: "test-session-token"}, nil
	})
	return p
}

func TestNewAnthropic_ClaudePlatform(t *testing.T) {
	t.Parallel()

	t.Run("mutually exclusive with bedrock", func(t *testing.T) {
		t.Parallel()

		_, err := NewAnthropic(context.Background(), config.Anthropic{}, &config.AWSBedrock{
			Region:         "us-west-2",
			Model:          "beddel",
			SmallFastModel: "modrock",
		}, claudePlatformIAMCfg())
		require.ErrorContains(t, err, "mutually exclusive")
	})

	t.Run("invalid config rejected", func(t *testing.T) {
		t.Parallel()

		cpCfg := claudePlatformIAMCfg()
		cpCfg.WorkspaceID = ""

		_, err := NewAnthropic(context.Background(), config.Anthropic{}, nil, cpCfg)
		require.ErrorContains(t, err, "workspace id required")
	})

	t.Run("ambient IAM credentials are lazy", func(t *testing.T) {
		t.Parallel()
		p, err := NewAnthropic(context.Background(), config.Anthropic{}, nil, claudePlatformIAMCfg())
		require.NoError(t, err)
		require.NotNil(t, p.claudePlatform)
		require.NotNil(t, p.claudePlatform.creds)
	})
}

func TestAnthropic_ClaudePlatformBaseURL(t *testing.T) {
	t.Parallel()

	t.Run("regional default", func(t *testing.T) {
		t.Parallel()

		p := newTestClaudePlatform(t, config.Anthropic{}, claudePlatformIAMCfg())
		require.Equal(t, "https://aws-external-anthropic.us-west-2.api.aws", p.BaseURL())
	})

	t.Run("override wins over anthropic base url", func(t *testing.T) {
		t.Parallel()

		cpCfg := claudePlatformIAMCfg()
		cpCfg.BaseURL = "https://proxy.internal"

		// The Claude Platform base URL is authoritative: the generic Anthropic
		// base URL is not consulted, so both bridged and passthrough routes
		// reach the same host.
		p := newTestClaudePlatform(t, config.Anthropic{BaseURL: "https://api.anthropic.com"}, cpCfg)
		require.Equal(t, "https://proxy.internal", p.BaseURL())
	})
}

//nolint:tparallel // The BYOK subtest changes AWS environment variables.
func TestAnthropic_ClaudePlatformResolveCredential(t *testing.T) {
	t.Run("ambient IAM signs when no key is present", func(t *testing.T) {
		t.Parallel()
		p := newTestClaudePlatform(t, config.Anthropic{}, claudePlatformIAMCfg())
		cred, err := p.resolveCredential(httptest.NewRequest(http.MethodPost, routeMessages, nil))
		require.NoError(t, err)
		require.IsType(t, intercept.AWSSigV4{}, cred)
	})

	t.Run("BYOK takes precedence without loading AWS", func(t *testing.T) {
		t.Setenv("AWS_PROFILE", "missing-profile")
		t.Setenv("AWS_CONFIG_FILE", "/dev/null")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/dev/null")
		p, err := NewAnthropic(context.Background(), config.Anthropic{}, nil, claudePlatformIAMCfg())
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, routeMessages, nil)
		req.Header.Set(intercept.AuthHeaderXAPIKey, "user-key")
		cred, err := p.resolveCredential(req)
		require.NoError(t, err)
		require.Equal(t, intercept.BYOK{Secret: "user-key", Header: intercept.AuthHeaderXAPIKey}, cred)
		inner := &captureTransport{}
		p.claudePlatform.inner = inner
		resp, err := p.claudePlatform.RoundTrip(req)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, "user-key", inner.req.Header.Get(intercept.AuthHeaderXAPIKey))
		require.Empty(t, inner.req.Header.Get(intercept.AuthHeaderAuthorization))
	})

	t.Run("pool is selected before ambient IAM", func(t *testing.T) {
		t.Parallel()
		p := newTestClaudePlatform(t, config.Anthropic{KeyPool: testutil.SingleKeyPool(config.ProviderAnthropic, "workspace-key")}, claudePlatformIAMCfg())
		cred, err := p.resolveCredential(httptest.NewRequest(http.MethodPost, routeMessages, nil))
		require.NoError(t, err)
		require.IsType(t, &intercept.CentralizedPool{}, cred)
	})
}

// captureTransport records the last request it saw and returns an empty 200.
type captureTransport struct {
	req *http.Request
}

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.req = req
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       http.NoBody,
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

type trackingBody struct {
	io.Reader
	closed bool
}

func (b *trackingBody) Close() error {
	b.closed = true
	return nil
}

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestAnthropic_ClaudePlatformCredentialInitializationRetries(t *testing.T) {
	p, err := NewAnthropic(context.Background(), config.Anthropic{}, nil, claudePlatformIAMCfg())
	require.NoError(t, err)
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_PROFILE", "missing-profile")
	t.Setenv("AWS_CONFIG_FILE", "/dev/null")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/dev/null")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	_, err = p.claudePlatform.creds.Retrieve(context.Background())
	require.ErrorContains(t, err, "build claude platform AWS credentials:")
	var profileErr awsconfig.SharedConfigProfileNotExistError
	require.ErrorAs(t, err, &profileErr)
	require.Equal(t, "missing-profile", profileErr.Profile)

	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")
	t.Setenv("AWS_SESSION_TOKEN", "test-session-token")
	creds, err := p.claudePlatform.creds.Retrieve(context.Background())
	require.NoError(t, err)
	require.Equal(t, "test-access-key", creds.AccessKeyID)
	require.Equal(t, "test-secret-key", creds.SecretAccessKey)
	require.Equal(t, "test-session-token", creds.SessionToken)

	// A successful load stays cached even if subsequent AWS configuration fails.
	t.Setenv("AWS_PROFILE", "missing-profile")
	cached, err := p.claudePlatform.creds.Retrieve(t.Context())
	require.NoError(t, err)
	require.Equal(t, creds, cached)
}

func TestAnthropic_ClaudePlatformCredentialResolutionContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		callerTimeout time.Duration
		cancel        bool
		wantErr       error
	}{
		{name: "caller cancellation", cancel: true, wantErr: context.Canceled},
		{name: "resolution deadline", wantErr: context.DeadlineExceeded},
		{name: "earlier caller deadline", callerTimeout: 5 * time.Second, wantErr: context.DeadlineExceeded},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				called := make(chan context.Context, 1)
				p := newTestClaudePlatform(t, config.Anthropic{}, claudePlatformIAMCfg())
				p.claudePlatform.creds = aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
					called <- ctx
					<-ctx.Done()
					return aws.Credentials{}, ctx.Err()
				})

				ctx := context.Background()
				var cancel context.CancelFunc
				if tt.callerTimeout > 0 {
					//nolint:gocritic // Simulated deadline tests precedence over the credential timeout.
					ctx, cancel = context.WithTimeout(ctx, tt.callerTimeout)
				} else {
					ctx, cancel = context.WithCancel(ctx)
				}
				defer cancel()

				body := &trackingBody{Reader: strings.NewReader("request body")}
				req := httptest.NewRequestWithContext(ctx, http.MethodGet, "https://aws-external-anthropic.us-west-2.api.aws/v1/models", body)
				result := make(chan error, 1)
				go func() {
					resp, err := p.WrapPassthroughTransport(&captureTransport{}).RoundTrip(req)
					if resp != nil {
						_ = resp.Body.Close()
					}
					result <- err
				}()

				resolvedCtx := <-called
				deadline, ok := resolvedCtx.Deadline()
				require.True(t, ok)
				remaining := time.Until(deadline)
				require.Greater(t, remaining, time.Duration(0))
				require.LessOrEqual(t, remaining, 30*time.Second)
				if tt.callerTimeout > 0 {
					require.LessOrEqual(t, remaining, tt.callerTimeout)
				}

				switch {
				case tt.cancel:
					cancel()
				case tt.callerTimeout > 0:
					<-time.After(tt.callerTimeout)
				default:
					<-time.After(30 * time.Second)
				}
				require.ErrorIs(t, <-result, tt.wantErr)
				require.True(t, body.closed, "request body must be closed when credential resolution fails")
			})
		})
	}
}

func TestAnthropic_ClaudePlatformStreamSurvivesCredentialDeadline(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		reader, writer := io.Pipe()
		t.Cleanup(func() {
			_ = reader.Close()
			_ = writer.Close()
		})
		var requestCtx context.Context
		var stop func() bool
		transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestCtx = req.Context()
			stop = context.AfterFunc(req.Context(), func() {
				_ = reader.CloseWithError(req.Context().Err())
			})
			if req.Body != nil {
				_ = req.Body.Close()
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       reader,
				Request:    req,
			}, nil
		})
		t.Cleanup(func() {
			if stop != nil {
				stop()
			}
		})
		p := newTestClaudePlatform(t, config.Anthropic{}, claudePlatformIAMCfg())
		p.claudePlatform.inner = transport
		service := anthropic.NewMessageService(
			option.WithBaseURL(p.BaseURL()),
			option.WithHTTPClient(&http.Client{Transport: p.claudePlatform}),
		)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		stream := service.NewStreaming(ctx, anthropic.MessageNewParams{
			Model:     "claude-opus-4-8",
			MaxTokens: 1,
			Messages: []anthropic.MessageParam{{
				Role:    anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock("hello")},
			}},
		})
		done := make(chan bool, 1)
		go func() { done <- stream.Next() }()
		<-time.After(31 * time.Second)
		synctest.Wait()
		require.NotNil(t, requestCtx)
		require.NoError(t, requestCtx.Err(), "outgoing streaming request context must remain live")
		_, err := writer.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{}}\n\n"))
		require.NoError(t, err)
		require.True(t, <-done)
		require.NoError(t, stream.Err())
		require.NoError(t, stream.Close())
	})
}

func TestAnthropic_WrapPassthroughTransport(t *testing.T) {
	t.Parallel()

	t.Run("not wrapped for plain anthropic", func(t *testing.T) {
		t.Parallel()

		p := newTestAnthropic(t, config.Anthropic{}, nil)
		inner := &captureTransport{}
		require.Same(t, http.RoundTripper(inner), p.WrapPassthroughTransport(inner))
	})

	t.Run("not wrapped for bedrock", func(t *testing.T) {
		t.Parallel()

		p := newTestAnthropic(t, config.Anthropic{}, &config.AWSBedrock{
			Region:         "us-west-2",
			Model:          "beddel",
			SmallFastModel: "modrock",
		})
		inner := &captureTransport{}
		require.Same(t, http.RoundTripper(inner), p.WrapPassthroughTransport(inner))
	})

	t.Run("iam signs and re-signs a reused request", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			p := newTestClaudePlatform(t, config.Anthropic{}, claudePlatformIAMCfg())
			inner := &captureTransport{}
			wrapped := p.WrapPassthroughTransport(inner)
			req := httptest.NewRequest(http.MethodGet, "https://aws-external-anthropic.us-west-2.api.aws/v1/models", nil)
			req.Header.Set(intercept.HeaderAnthropicWorkspaceID, "wrkspc_from_client")
			original := req.Header.Clone()

			resp, err := wrapped.RoundTrip(req)
			require.NoError(t, err)
			_ = resp.Body.Close()
			require.NotNil(t, inner.req)
			firstAuth := inner.req.Header.Get(intercept.AuthHeaderAuthorization)
			firstDate := inner.req.Header.Get("X-Amz-Date")
			require.Equal(t, "wrkspc_config", inner.req.Header.Get(intercept.HeaderAnthropicWorkspaceID),
				"provider configuration must own the workspace ID")
			require.True(t, strings.HasPrefix(firstAuth, "AWS4-HMAC-SHA256"), "missing SigV4 auth: %q", firstAuth)
			require.Contains(t, firstAuth, "/aws-external-anthropic/aws4_request",
				"signature must be scoped to the aws-external-anthropic service")

			<-time.After(2 * time.Second)
			resp, err = wrapped.RoundTrip(req)
			require.NoError(t, err)
			_ = resp.Body.Close()
			require.NotNil(t, inner.req)
			require.NotEqual(t, firstAuth, inner.req.Header.Get(intercept.AuthHeaderAuthorization))
			require.NotEqual(t, firstDate, inner.req.Header.Get("X-Amz-Date"))
			require.Equal(t, "wrkspc_config", inner.req.Header.Get(intercept.HeaderAnthropicWorkspaceID))
			require.Equal(t, original, req.Header, "signing must not mutate the caller request")
		})
	})

	t.Run("byok is forwarded unsigned", func(t *testing.T) {
		t.Parallel()

		p := newTestClaudePlatform(t, config.Anthropic{}, claudePlatformIAMCfg())
		called := false
		p.claudePlatform.creds = aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			called = true
			return aws.Credentials{}, context.Canceled
		})
		inner := &captureTransport{}

		req := httptest.NewRequest(http.MethodGet, "https://aws-external-anthropic.us-west-2.api.aws/v1/models", nil)
		req.Header.Set(intercept.AuthHeaderXAPIKey, "user-key")

		resp, err := p.WrapPassthroughTransport(inner).RoundTrip(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.NotNil(t, inner.req)
		require.False(t, called, "BYOK must not resolve IAM credentials")
		require.Equal(t, "user-key", inner.req.Header.Get(intercept.AuthHeaderXAPIKey))
		require.Empty(t, inner.req.Header.Get(intercept.AuthHeaderAuthorization),
			"a request that already carries a credential must not also be signed")
		require.Equal(t, "wrkspc_config", inner.req.Header.Get(intercept.HeaderAnthropicWorkspaceID))
	})

	for _, transport := range []string{"messages", "passthrough"} {
		for _, tt := range []struct {
			name, header, credential, authPath string
		}{
			{name: "ambient IAM", authPath: "aws_sigv4"},
			{name: "API key", header: intercept.AuthHeaderXAPIKey, credential: "test-api-key", authPath: "existing_credential"},
			{name: "bearer token", header: intercept.AuthHeaderAuthorization, credential: "Bearer test-bearer-token", authPath: "existing_credential"},
		} {
			for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo} {
				t.Run(transport+"/"+tt.name+"/"+level.String(), func(t *testing.T) {
					t.Parallel()
					var logs bytes.Buffer
					logger := slog.Make(sloghuman.Sink(&logs)).Leveled(level)
					p := newTestClaudePlatform(t, config.Anthropic{Logger: logger}, claudePlatformIAMCfg())
					inner := &captureTransport{}
					p.claudePlatform.inner = inner
					var wrapped http.RoundTripper = p.claudePlatform
					if transport == "passthrough" {
						wrapped = p.WrapPassthroughTransport(inner)
					}

					ctx := slog.With(t.Context(), slog.F("request_id", "test-request-id"))
					req := httptest.NewRequestWithContext(ctx, http.MethodGet, p.BaseURL()+"/v1/models", nil)
					if tt.header != "" {
						req.Header.Set(tt.header, tt.credential)
					}
					resp, err := wrapped.RoundTrip(req)
					require.NoError(t, err)
					require.NoError(t, resp.Body.Close())
					require.Equal(t, "wrkspc_config", inner.req.Header.Get(intercept.HeaderAnthropicWorkspaceID))
					if tt.header == "" {
						require.True(t, strings.HasPrefix(inner.req.Header.Get(intercept.AuthHeaderAuthorization), "AWS4-HMAC-SHA256"))
					} else {
						require.Equal(t, tt.credential, inner.req.Header.Get(tt.header))
					}

					if level == slog.LevelInfo {
						require.Empty(t, logs.String(), "authentication diagnostics must be debug-only")
						return
					}
					require.Contains(t, logs.String(), "claude platform authentication")
					require.Contains(t, logs.String(), "auth_path="+tt.authPath)
					require.Contains(t, logs.String(), "request_id=test-request-id")
					for _, secret := range []string{"test-api-key", "test-bearer-token", "test-access-key", "test-secret-key", "test-session-token", "Signature="} {
						require.NotContains(t, logs.String(), secret)
					}
				})
			}
		}
	}
}
