// Package awssig holds the shared AWS SigV4 signing and Bedrock Mantle
// routing helpers used by both the anthropic-go and openai-go interceptors.
// It depends only on stdlib and the AWS SDK so it can be imported from either
// SDK-specific interceptor package without pulling in the other SDK.
package awssig

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"golang.org/x/xerrors"
)

// ServiceBedrockMantle is the AWS SigV4 service name for Bedrock Mantle.
const ServiceBedrockMantle = "bedrock-mantle"

// PRMUserAgent is Coder's AWS Partner Revenue Measurement (PRM) attribution
// marker for outbound Bedrock requests. It is appended to the User-Agent
// header so AWS can recognize the traffic as Coder-associated Bedrock usage.
//
// It is Bedrock-specific: only the Bedrock option builders apply it. Other
// AWS-signed endpoints must not send it without their own attribution
// agreement.
const PRMUserAgent = "sdk-ua-app-id/APN_1.1%2Fpc_cdfmjwn8i6u8l9fwz8h82e4w3%24"

// AppendPRMUserAgent appends the Coder PRM attribution marker to the request's
// User-Agent header.
func AppendPRMUserAgent(req *http.Request) {
	if ua := req.Header.Get("User-Agent"); ua != "" {
		req.Header.Set("User-Agent", ua+" "+PRMUserAgent)
	}
}

// SignMiddleware returns an stdlib HTTP middleware that SigV4-signs the request
// for the named AWS service. It reads and restores the body for hashing, then
// signs with the given credentials and region. Callers wrap it in their
// SDK-specific option.WithMiddleware adapter.
//
// The caller is responsible for any service-specific headers or user-agent
// attribution; this middleware only signs.
func SignMiddleware(creds aws.CredentialsProvider, region, service string) func(req *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	signer := v4.NewSigner()
	return func(req *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
		resolved, err := creds.Retrieve(req.Context())
		if err != nil {
			return nil, xerrors.Errorf("%s SigV4: resolve AWS credentials: %w", service, err)
		}

		// SigV4 requires a payload hash, so read the body to hash it and then
		// restore it for the downstream HTTP client to send.
		var body []byte
		if req.Body != nil {
			body, err = io.ReadAll(req.Body)
			if err != nil {
				return nil, xerrors.Errorf("%s SigV4: read request body: %w", service, err)
			}
			_ = req.Body.Close()
			req.Body = io.NopCloser(bytes.NewReader(body))
			req.ContentLength = int64(len(body))
		}

		hash := sha256.Sum256(body)
		if err := signer.SignHTTP(req.Context(), resolved, req, hex.EncodeToString(hash[:]), service, region, time.Now()); err != nil {
			return nil, xerrors.Errorf("%s SigV4: sign request: %w", service, err)
		}
		return next(req)
	}
}

// trimMantleSegments removes any trailing Mantle vendor path segments from the
// parsed URL path so BaseURLForModel can re-append the correct one. Applied in
// longest-first order so /anthropic/v1 and /openai/v1 collapse before their
// bare vendor segments.
func trimMantleSegments(path string) string {
	path = strings.TrimSuffix(path, "/")
	path = strings.TrimSuffix(path, "/anthropic/v1")
	path = strings.TrimSuffix(path, "/openai/v1")
	path = strings.TrimSuffix(path, "/anthropic")
	path = strings.TrimSuffix(path, "/openai")
	return path
}

// BaseURLForModel resolves the upstream Mantle base URL for a given model. It
// parses rawBase, trims any existing vendor path segment, then re-appends the
// one the model requires: "anthropic." models use /anthropic, "openai." models
// use /openai/v1, and anything else (third-party) uses /v1 so the SDK's
// /chat/completions route hits the root /v1/chat/completions endpoint.
func BaseURLForModel(rawBase, model string) (string, error) {
	u, err := url.Parse(rawBase)
	if err != nil {
		return "", xerrors.Errorf("mantle base URL: %w", err)
	}

	u.Path = trimMantleSegments(u.Path)

	switch {
	case strings.HasPrefix(model, "anthropic."):
		u.Path += "/anthropic"
	case strings.HasPrefix(model, "openai."):
		u.Path += "/openai/v1"
	default:
		// Third-party models use the root /v1 path segment.
		u.Path += "/v1"
	}
	return u.String(), nil
}
