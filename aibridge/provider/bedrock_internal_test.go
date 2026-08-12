package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/config"
)

// TestBuildBedrockCredentialsValidation covers the input validation that does
// not require resolving credentials.
func TestBuildBedrockCredentialsValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cfg      config.AWSBedrock
		errorMsg string
	}{
		{
			name:     "missing region and base url",
			cfg:      config.AWSBedrock{},
			errorMsg: "region or base url required",
		},
		{
			name: "missing access key",
			cfg: config.AWSBedrock{
				Region:          "us-east-1",
				AccessKeySecret: "test-secret",
			},
			errorMsg: "both access key and access key secret must be provided together",
		},
		{
			name: "missing access key secret",
			cfg: config.AWSBedrock{
				Region:    "us-east-1",
				AccessKey: "test-key",
			},
			errorMsg: "both access key and access key secret must be provided together",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := buildBedrockCredentials(context.Background(), tt.cfg)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.errorMsg)
		})
	}
}

// TestBuildBedrockCredentialsStatic resolves static credentials.
func TestBuildBedrockCredentialsStatic(t *testing.T) {
	t.Parallel()

	creds, _, err := buildBedrockCredentials(context.Background(), config.AWSBedrock{
		Region:          "us-east-1",
		AccessKey:       "test-key",
		AccessKeySecret: "test-secret",
	})
	require.NoError(t, err)

	got, err := creds.Retrieve(context.Background())
	require.NoError(t, err)
	require.Equal(t, "test-key", got.AccessKeyID)
	require.Equal(t, "test-secret", got.SecretAccessKey)
}

// TestBuildBedrockCredentialsDefaultChain covers resolution via the AWS SDK
// default credential chain.
// NOTE: no t.Parallel() because the subtests use t.Setenv.
func TestBuildBedrockCredentialsDefaultChain(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		expectError bool
		wantKey     string
		wantSecret  string
		wantToken   string
	}{
		{
			name: "credentials via env",
			envVars: map[string]string{
				"AWS_ACCESS_KEY_ID":     "test-key",
				"AWS_SECRET_ACCESS_KEY": "test-secret",
			},
			wantKey:    "test-key",
			wantSecret: "test-secret",
		},
		{
			name: "credentials with session token via env",
			envVars: map[string]string{
				"AWS_ACCESS_KEY_ID":     "test-key",
				"AWS_SECRET_ACCESS_KEY": "test-secret",
				"AWS_SESSION_TOKEN":     "test-session-token",
			},
			wantKey:    "test-key",
			wantSecret: "test-secret",
			wantToken:  "test-session-token",
		},
		{
			name: "error when no credential source is configured",
			envVars: map[string]string{
				"AWS_ACCESS_KEY_ID":                      "",
				"AWS_SECRET_ACCESS_KEY":                  "",
				"AWS_SESSION_TOKEN":                      "",
				"AWS_PROFILE":                            "",
				"AWS_SHARED_CREDENTIALS_FILE":            "/dev/null",
				"AWS_CONFIG_FILE":                        "/dev/null",
				"AWS_WEB_IDENTITY_TOKEN_FILE":            "",
				"AWS_ROLE_ARN":                           "",
				"AWS_ROLE_SESSION_NAME":                  "",
				"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI": "",
				"AWS_CONTAINER_CREDENTIALS_FULL_URI":     "",
				"AWS_CONTAINER_AUTHORIZATION_TOKEN":      "",
				"AWS_EC2_METADATA_DISABLED":              "true",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, val := range tt.envVars {
				t.Setenv(key, val)
			}

			// buildBedrockCredentials only wires up the provider chain; it
			// does not resolve credentials, so it succeeds regardless of
			// credential availability. Resolution failures surface on Retrieve.
			creds, _, err := buildBedrockCredentials(context.Background(), config.AWSBedrock{
				Region: "us-east-1",
			})
			require.NoError(t, err)
			require.NotNil(t, creds)

			got, err := creds.Retrieve(context.Background())
			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantKey, got.AccessKeyID)
			require.Equal(t, tt.wantSecret, got.SecretAccessKey)
			require.Equal(t, tt.wantToken, got.SessionToken)
		})
	}
}

// TestBuildBedrockCredentialsAssumeRole drives the STS AssumeRole path against a
// mock endpoint, asserting that the configured role ARN and the stable session
// name are sent and that the returned temporary credentials are used.
// NOTE: no t.Parallel() because it uses t.Setenv.
func TestBuildBedrockCredentialsAssumeRole(t *testing.T) {
	var gotRoleARN, gotSessionName, gotConnection string
	// Mock the AWS STS AssumeRole API.
	// https://docs.aws.amazon.com/STS/latest/APIReference/API_AssumeRole.html
	sts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		gotRoleARN = r.Form.Get("RoleArn")
		gotSessionName = r.Form.Get("RoleSessionName")
		// With keep-alive disabled, Go's HTTP client sends Connection: close.
		gotConnection = r.Header.Get("Connection")

		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleResult>
    <Credentials>
      <AccessKeyId>ASIAASSUMED</AccessKeyId>
      <SecretAccessKey>assumed-secret</SecretAccessKey>
      <SessionToken>assumed-token</SessionToken>
      <Expiration>2999-01-01T00:00:00Z</Expiration>
    </Credentials>
    <AssumedRoleUser>
      <Arn>arn:aws:sts::123456789012:assumed-role/target/coder</Arn>
      <AssumedRoleId>AROAEXAMPLE:coder</AssumedRoleId>
    </AssumedRoleUser>
  </AssumeRoleResult>
</AssumeRoleResponse>`))
	}))
	defer sts.Close()

	// Point the STS client at the mock and provide static base credentials so
	// the base identity resolves without additional network calls.
	t.Setenv("AWS_ENDPOINT_URL_STS", sts.URL)
	t.Setenv("AWS_ACCESS_KEY_ID", "base-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "base-secret")

	creds, _, err := buildBedrockCredentials(context.Background(), config.AWSBedrock{
		Region:  "us-east-1",
		RoleARN: "arn:aws:iam::123456789012:role/target",
	})
	require.NoError(t, err)

	got, err := creds.Retrieve(context.Background())
	require.NoError(t, err)
	require.Equal(t, "ASIAASSUMED", got.AccessKeyID)
	require.Equal(t, "assumed-secret", got.SecretAccessKey)
	require.Equal(t, "assumed-token", got.SessionToken)

	require.Equal(t, "arn:aws:iam::123456789012:role/target", gotRoleARN)
	require.Equal(t, bedrockSessionName, gotSessionName)
	// The STS client disables keep-alive so each AssumeRole opens a fresh
	// connection; Go signals this with a Connection: close request header.
	require.Equal(t, "close", gotConnection,
		"STS client should disable keep-alives so each AssumeRole opens a fresh connection")
}

// TestBuildBedrockCredentialsAssumeRoleExternalID verifies that a configured
// external ID is sent on the STS AssumeRole call, and that omitting it sends
// no ExternalId parameter.
// NOTE: no t.Parallel() because it uses t.Setenv.
func TestBuildBedrockCredentialsAssumeRoleExternalID(t *testing.T) {
	tests := []struct {
		name           string
		externalID     string
		wantExternalID string
	}{
		{name: "with external id", externalID: "trust-policy-id-123", wantExternalID: "trust-policy-id-123"},
		{name: "without external id", externalID: "", wantExternalID: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotExternalID string
			// Mock the AWS STS AssumeRole API.
			// https://docs.aws.amazon.com/STS/latest/APIReference/API_AssumeRole.html
			sts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.NoError(t, r.ParseForm())
				gotExternalID = r.Form.Get("ExternalId")

				w.Header().Set("Content-Type", "text/xml")
				_, _ = w.Write([]byte(`<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleResult>
    <Credentials>
      <AccessKeyId>ASIAASSUMED</AccessKeyId>
      <SecretAccessKey>assumed-secret</SecretAccessKey>
      <SessionToken>assumed-token</SessionToken>
      <Expiration>2999-01-01T00:00:00Z</Expiration>
    </Credentials>
  </AssumeRoleResult>
</AssumeRoleResponse>`))
			}))
			defer sts.Close()

			t.Setenv("AWS_ENDPOINT_URL_STS", sts.URL)
			t.Setenv("AWS_ACCESS_KEY_ID", "base-key")
			t.Setenv("AWS_SECRET_ACCESS_KEY", "base-secret")

			creds, _, err := buildBedrockCredentials(context.Background(), config.AWSBedrock{
				Region:     "us-east-1",
				RoleARN:    "arn:aws:iam::123456789012:role/target",
				ExternalID: tt.externalID,
			})
			require.NoError(t, err)

			_, err = creds.Retrieve(context.Background())
			require.NoError(t, err)
			require.Equal(t, tt.wantExternalID, gotExternalID)
		})
	}
}

// TestBuildBedrockCredentialsAssumeRoleError verifies that when STS rejects the
// AssumeRole call (e.g. a trust-policy or IAM denial), the failure surfaces to
// the caller on Retrieve with enough detail to diagnose it, rather than being
// swallowed. The base identity resolved fine; only the role assumption failed.
// NOTE: no t.Parallel() because it uses t.Setenv.
func TestBuildBedrockCredentialsAssumeRoleError(t *testing.T) {
	// Mock the AWS STS AssumeRole API.
	// https://docs.aws.amazon.com/STS/latest/APIReference/API_AssumeRole.html
	sts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<ErrorResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <Error>
    <Type>Sender</Type>
    <Code>AccessDenied</Code>
    <Message>User arn:aws:iam::123456789012:user/base is not authorized to perform sts:AssumeRole on arn:aws:iam::123456789012:role/target</Message>
  </Error>
</ErrorResponse>`))
	}))
	defer sts.Close()

	t.Setenv("AWS_ENDPOINT_URL_STS", sts.URL)
	t.Setenv("AWS_ACCESS_KEY_ID", "base-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "base-secret")

	creds, _, err := buildBedrockCredentials(context.Background(), config.AWSBedrock{
		Region:  "us-east-1",
		RoleARN: "arn:aws:iam::123456789012:role/target",
	})
	require.NoError(t, err) // Build is lazy; the STS call happens on Retrieve.

	_, err = creds.Retrieve(context.Background())
	require.Error(t, err)
	// The error must carry the STS operation and failure code so operators can
	// tell this is an AssumeRole authorization problem, not missing credentials.
	require.ErrorContains(t, err, "AssumeRole")
	require.ErrorContains(t, err, "AccessDenied")
}

// TestBuildBedrockCredentialsAssumeRoleCaches verifies the AssumeRole result is
// cached: many credential retrievals, one per LLM request, trigger a single STS
// AssumeRole call rather than re-assuming the role on every request.
// NOTE: no t.Parallel() because it uses t.Setenv.
func TestBuildBedrockCredentialsAssumeRoleCaches(t *testing.T) {
	var stsCalls atomic.Int64
	// Mock the AWS STS AssumeRole API.
	// https://docs.aws.amazon.com/STS/latest/APIReference/API_AssumeRole.html
	sts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		stsCalls.Add(1)
		w.Header().Set("Content-Type", "text/xml")
		// A far-future expiration keeps the cached credentials valid, so the
		// cache serves every retrieval after the first without re-assuming.
		_, _ = w.Write([]byte(`<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleResult>
    <Credentials>
      <AccessKeyId>ASIAASSUMED</AccessKeyId>
      <SecretAccessKey>assumed-secret</SecretAccessKey>
      <SessionToken>assumed-token</SessionToken>
      <Expiration>2999-01-01T00:00:00Z</Expiration>
    </Credentials>
  </AssumeRoleResult>
</AssumeRoleResponse>`))
	}))
	defer sts.Close()

	t.Setenv("AWS_ENDPOINT_URL_STS", sts.URL)
	t.Setenv("AWS_ACCESS_KEY_ID", "base-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "base-secret")

	creds, _, err := buildBedrockCredentials(context.Background(), config.AWSBedrock{
		Region:  "us-east-1",
		RoleARN: "arn:aws:iam::123456789012:role/target",
	})
	require.NoError(t, err)

	// Each retrieval stands in for an LLM request resolving credentials from the
	// shared provider. Only the first should reach STS.
	for range 5 {
		got, err := creds.Retrieve(context.Background())
		require.NoError(t, err)
		require.Equal(t, "ASIAASSUMED", got.AccessKeyID)
	}

	require.Equal(t, int64(1), stsCalls.Load(),
		"AssumeRole should be called once, then served from the credentials cache")
}

// TestBuildBedrockCredentialsAssumeRoleRefreshesOnExpiry verifies that once the
// assumed credentials expire, the next retrieval re-assumes the role rather than
// serving stale credentials from the cache.
// NOTE: no t.Parallel() because it uses t.Setenv.
func TestBuildBedrockCredentialsAssumeRoleRefreshesOnExpiry(t *testing.T) {
	var stsCalls atomic.Int64
	// Mock the AWS STS AssumeRole API.
	// https://docs.aws.amazon.com/STS/latest/APIReference/API_AssumeRole.html
	sts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		stsCalls.Add(1)
		w.Header().Set("Content-Type", "text/xml")
		// An expiration in the past makes the returned credentials immediately
		// stale, so the cache cannot reuse them and must re-assume on the next
		// retrieval.
		_, _ = w.Write([]byte(`<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleResult>
    <Credentials>
      <AccessKeyId>ASIAASSUMED</AccessKeyId>
      <SecretAccessKey>assumed-secret</SecretAccessKey>
      <SessionToken>assumed-token</SessionToken>
      <Expiration>2000-01-01T00:00:00Z</Expiration>
    </Credentials>
  </AssumeRoleResult>
</AssumeRoleResponse>`))
	}))
	defer sts.Close()

	t.Setenv("AWS_ENDPOINT_URL_STS", sts.URL)
	t.Setenv("AWS_ACCESS_KEY_ID", "base-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "base-secret")

	creds, _, err := buildBedrockCredentials(context.Background(), config.AWSBedrock{
		Region:  "us-east-1",
		RoleARN: "arn:aws:iam::123456789012:role/target",
	})
	require.NoError(t, err)

	_, err = creds.Retrieve(context.Background())
	require.NoError(t, err)
	_, err = creds.Retrieve(context.Background())
	require.NoError(t, err)

	require.Equal(t, int64(2), stsCalls.Load(),
		"expired credentials should trigger a fresh AssumeRole on the next retrieval")
}

// TestBuildBedrockCredentialsAssumeRoleRequiresRegion verifies that configuring
// a role without a resolvable region fails at construction. STS needs a region
// to resolve its endpoint.
// NOTE: no t.Parallel() because it uses t.Setenv.
func TestBuildBedrockCredentialsAssumeRoleRequiresRegion(t *testing.T) {
	// Ensure no region resolves from the environment, shared config, or IMDS,
	// so base.Region ends up empty.
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_CONFIG_FILE", "/dev/null")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/dev/null")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	_, _, err := buildBedrockCredentials(context.Background(), config.AWSBedrock{
		BaseURL: "https://bedrock-runtime.example.com",
		RoleARN: "arn:aws:iam::123456789012:role/target",
	})
	require.ErrorContains(t, err, "region is required to assume a role")
}

// TestBuildBedrockCredentialsAssumeRoleRegionFromEnv verifies that a role
// configured without an explicit region resolves it from the AWS environment
// (AWS_REGION here).
// NOTE: no t.Parallel() because it uses t.Setenv.
func TestBuildBedrockCredentialsAssumeRoleRegionFromEnv(t *testing.T) {
	t.Setenv("AWS_REGION", "us-west-2")

	// BaseURL set with no explicit region: the region comes from AWS_REGION.
	_, region, err := buildBedrockCredentials(context.Background(), config.AWSBedrock{
		BaseURL: "https://bedrock-runtime.example.com",
		RoleARN: "arn:aws:iam::123456789012:role/target",
	})
	require.NoError(t, err)
	require.Equal(t, "us-west-2", region)
}
