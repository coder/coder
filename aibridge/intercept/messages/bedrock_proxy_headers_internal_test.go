package messages

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
)

// TestBlockingInterception_BedrockProxyHeadersBreakSigV4 reproduces a
// production failure mode reported by a customer running AI Bridge behind
// a reverse-proxy chain that targets AWS Bedrock.
//
// The chain that produced the failure was, roughly:
//
//	browser -> coder app proxy -> coderd (aibridge) -> [outbound hop?] -> Bedrock
//
// In the customer's deployment, the original client request arrived at
// aibridge carrying X-Forwarded-For / X-Forwarded-Host / X-Forwarded-Proto
// headers populated by the upstream Coder app proxy. aibridge's client-
// header forwarding middleware (intercept.BuildUpstreamHeaders) preserves
// every non-auth, non-hop-by-hop client header on the outbound request to
// the upstream provider. The Anthropic SDK's Bedrock middleware then signs
// the outgoing request with SigV4. AWS SigV4 signs every request header by
// default (the unsigned-headers denylist is small: User-Agent, Authorization,
// Expect, etc.), so X-Forwarded-* end up in the canonical request and the
// SignedHeaders list.
//
// If anything between aibridge and bedrock-runtime.<region>.amazonaws.com
// mutates one of those signed headers (an outbound HTTP proxy that appends
// itself to X-Forwarded-For is the most common offender), AWS rejects the
// request with SignatureDoesNotMatch because the canonical request it
// reconstructs no longer matches the one aibridge signed.
//
// This test sets up:
//
//	aibridge BlockingInterception -> mockProxy (mutates X-Forwarded-For)
//	                              -> mockBedrock (verifies SigV4 against the
//	                                 received headers using the same creds)
//
// We assert two things:
//
//  1. A baseline request with no mutating proxy verifies successfully:
//     aibridge -> mockBedrock yields a 200. This proves the mock SigV4
//     verifier agrees with the SDK's signer.
//  2. The same request through the mutating proxy fails with the AWS
//     SignatureDoesNotMatch error shape, demonstrating that any in-path
//     mutation of a header that ended up in SignedHeaders breaks auth.
func TestBlockingInterception_BedrockProxyHeadersBreakSigV4(t *testing.T) {
	t.Parallel()

	const (
		accessKey = "AKIAIOSFODNN7EXAMPLE"
		secretKey = "wJalrXUtnFEMI/K7MDENG/bCxEFICAYEXAMPLEKEY"
		region    = "us-east-2"
		model     = "us.anthropic.claude-opus-4-6-v1"
	)

	// mockBedrock validates SigV4 against the received request and returns
	// 200 + a non-streaming Anthropic Bedrock response when the signature
	// matches the headers it received, or 403 with an AWS-shaped error
	// otherwise.
	mockBedrock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(w, "read body: %v", err)
			return
		}

		if err := verifySigV4(r, body, accessKey, secretKey); err != nil {
			// Mimic the AWS error shape so debugging matches what
			// operators see in production.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"message": "The request signature we calculated does not match the signature you provided. " +
					"Check your AWS Secret Access Key and signing method. " + err.Error(),
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(bedrockProxyHeadersTestResponse())
	}))
	t.Cleanup(mockBedrock.Close)

	// mockProxy sits between aibridge and mockBedrock and appends to the
	// X-Forwarded-For header before forwarding. This mirrors a real outbound
	// HTTP proxy that adds its own hop to the chain.
	bedrockURL, err := url.Parse(mockBedrock.URL)
	require.NoError(t, err)

	mockProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Forward to the bedrock mock but mutate X-Forwarded-For first.
		// This is the entire shape of the bug: a header that was included
		// in the SigV4 SignedHeaders list is changed in flight.
		outURL := *bedrockURL
		outURL.Path = r.URL.Path
		outURL.RawQuery = r.URL.RawQuery
		outReq, err := http.NewRequestWithContext(r.Context(), r.Method, outURL.String(), strings.NewReader(string(body)))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		for k, vs := range r.Header {
			for _, v := range vs {
				outReq.Header.Add(k, v)
			}
		}
		// Mutate X-Forwarded-For in the exact shape a transparent HTTP
		// proxy would: append its own observed remote-addr.
		prior := outReq.Header.Get("X-Forwarded-For")
		if prior == "" {
			outReq.Header.Set("X-Forwarded-For", "10.0.0.42")
		} else {
			outReq.Header.Set("X-Forwarded-For", prior+", 10.0.0.42")
		}

		resp, err := http.DefaultClient.Do(outReq)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	t.Cleanup(mockProxy.Close)

	// Common interceptor invocation. We use the BaseURL to swap between
	// the direct-to-bedrock baseline and the mutating-proxy repro.
	runOnce := func(t *testing.T, baseURL string, clientHeaders http.Header) (int, string) {
		t.Helper()

		bedrockCfg := &config.AWSBedrock{
			BaseURL:         baseURL,
			Region:          region,
			AccessKey:       accessKey,
			AccessKeySecret: secretKey,
			Model:           model,
			SmallFastModel:  "anthropic.claude-haiku-3-5",
		}

		payload, err := NewRequestPayload([]byte(requestBody))
		require.NoError(t, err)

		interceptor := NewBlockingInterceptor(
			uuid.New(),
			payload,
			config.ProviderAnthropic,
			config.Anthropic{BaseURL: baseURL},
			bedrockCfg,
			clientHeaders,
			"X-Api-Key",
			otel.Tracer("bedrock_proxy_headers_test"),
			intercept.NewCredentialInfo(intercept.CredentialKindCentralized, ""),
		)
		interceptor.Setup(slog.Make(), &testutil.MockRecorder{}, nil)

		req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
		rec := httptest.NewRecorder()
		_ = interceptor.ProcessRequest(rec, req)
		respBody, _ := io.ReadAll(rec.Body)
		return rec.Code, string(respBody)
	}

	// Headers an app-proxy chain would inject before aibridge receives the
	// request. These mirror what produced the customer's canonical request
	// in the AWS error message.
	proxyClientHeaders := http.Header{
		"X-Forwarded-For":   {"3.146.178.71, 172.18.0.3"},
		"X-Forwarded-Host":  {"3ko007lttg5ko.pit-1.try.coder.app"},
		"X-Forwarded-Proto": {"https"},
	}

	t.Run("baseline_direct_to_bedrock_passes", func(t *testing.T) {
		t.Parallel()
		// Sanity check: with no proxy in between, the X-Forwarded-* headers
		// reach the bedrock mock unchanged and SigV4 verification succeeds.
		code, body := runOnce(t, mockBedrock.URL, proxyClientHeaders.Clone())
		require.Equalf(t, http.StatusOK, code, "expected baseline 200, got %d: %s", code, body)
		require.Contains(t, body, "Hi there")
	})

	t.Run("mutating_proxy_breaks_signature", func(t *testing.T) {
		t.Parallel()
		// The mutating proxy appends to X-Forwarded-For. Because that
		// header was in the SigV4 SignedHeaders list, the bedrock mock's
		// recomputed signature no longer matches what the SDK signed.
		code, body := runOnce(t, mockProxy.URL, proxyClientHeaders.Clone())
		assert.NotEqualf(t, http.StatusOK, code, "expected non-200 due to signature mismatch, got 200: %s", body)
		assert.Containsf(t, body, "does not match",
			"expected AWS-shaped SignatureDoesNotMatch error, got %d: %s", code, body)
	})
}

// verifySigV4 recomputes the SigV4 signature for the given request using the
// supplied access key + secret and compares it against the Authorization
// header on the request. It returns nil if and only if the two signatures
// match exactly. On mismatch the error includes both signatures and the
// SignedHeaders list, mirroring the diagnostic AWS returns in production.
//
// AWS performs the same verification on its side: it parses the
// Authorization header to extract the credential scope and SignedHeaders
// list, then independently rebuilds the canonical request and signing
// string from the headers it actually received. Any divergence between
// what the client signed and what AWS receives produces a
// SignatureDoesNotMatch response.
func verifySigV4(r *http.Request, body []byte, accessKey, secretKey string) error {
	authz := r.Header.Get("Authorization")
	if !strings.HasPrefix(authz, "AWS4-HMAC-SHA256 ") {
		return fmt.Errorf("missing or malformed Authorization header: %q", authz)
	}

	parts := map[string]string{}
	for _, kv := range strings.Split(strings.TrimPrefix(authz, "AWS4-HMAC-SHA256 "), ",") {
		kv = strings.TrimSpace(kv)
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		parts[kv[:eq]] = kv[eq+1:]
	}
	cred := parts["Credential"]
	signedHeadersList := parts["SignedHeaders"]
	gotSignature := parts["Signature"]
	if cred == "" || signedHeadersList == "" || gotSignature == "" {
		return fmt.Errorf("incomplete Authorization header: %q", authz)
	}

	credParts := strings.Split(cred, "/")
	if len(credParts) != 5 {
		return fmt.Errorf("malformed Credential scope %q", cred)
	}
	gotAccessKey, _, gotRegion, service := credParts[0], credParts[1], credParts[2], credParts[3]
	if gotAccessKey != accessKey {
		return fmt.Errorf("credential access key %q does not match expected %q", gotAccessKey, accessKey)
	}
	_ = gotSignature // included in the mismatch error below for diagnostics

	// Parse the signing time from X-Amz-Date so re-signing produces the
	// same value.
	signTimeStr := r.Header.Get("X-Amz-Date")
	signTime, err := time.Parse("20060102T150405Z", signTimeStr)
	if err != nil {
		return fmt.Errorf("parse X-Amz-Date %q: %w", signTimeStr, err)
	}

	// Build a fresh request that contains ONLY the headers that were
	// included in SignedHeaders, in the same order. The v4 signer
	// canonicalizes whatever headers are on the request, so leaving
	// non-signed headers in would cause a false mismatch in verification.
	signed := map[string]struct{}{}
	for _, h := range strings.Split(signedHeadersList, ";") {
		signed[strings.ToLower(h)] = struct{}{}
	}

	verifyReq, err := http.NewRequest(r.Method, "http://"+r.Host+r.URL.RequestURI(), strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("build verification request: %w", err)
	}
	for k, vs := range r.Header {
		if _, ok := signed[strings.ToLower(k)]; !ok {
			continue
		}
		for _, v := range vs {
			verifyReq.Header.Add(k, v)
		}
	}
	// http.Request.Host is signed via the "host" header in the canonical
	// request; ensure it matches what was used at signing time.
	verifyReq.Host = r.Host

	// Hash the body. Bedrock /invoke uses the standard payload hash.
	payloadHash := r.Header.Get("X-Amz-Content-Sha256")
	if payloadHash == "" {
		sum := sha256.Sum256(body)
		payloadHash = hex.EncodeToString(sum[:])
	}

	signer := v4.NewSigner()
	if err := signer.SignHTTP(context.Background(), aws.Credentials{
		AccessKeyID:     accessKey,
		SecretAccessKey: secretKey,
	}, verifyReq, payloadHash, service, gotRegion, signTime); err != nil {
		return fmt.Errorf("re-sign request: %w", err)
	}

	wantAuthz := verifyReq.Header.Get("Authorization")
	if wantAuthz == authz {
		return nil
	}
	return fmt.Errorf("signature mismatch: client sent %q, we recomputed %q (SignedHeaders=%q)",
		gotSignature, strings.TrimPrefix(wantAuthz, "AWS4-HMAC-SHA256 "), signedHeadersList)
}

// bedrockProxyHeadersTestResponse is the minimum response shape the
// Anthropic SDK's Bedrock middleware accepts for a non-streaming /invoke
// call. Defined locally so the test does not depend on chatprovider test
// helpers.
func bedrockProxyHeadersTestResponse() map[string]any {
	return map[string]any{
		"id":    "msg_01Test",
		"type":  "message",
		"role":  "assistant",
		"model": "claude-opus-4-6",
		"content": []any{
			map[string]any{"type": "text", "text": "Hi there"},
		},
		"stop_reason":   "end_turn",
		"stop_sequence": "",
		"usage": map[string]any{
			"input_tokens":  5,
			"output_tokens": 2,
		},
	}
}
