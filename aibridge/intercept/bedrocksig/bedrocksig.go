// Package bedrocksig holds the shared AWS SigV4 signing helpers for Bedrock
// Mantle. It depends only on stdlib and the AWS SDK so it can be imported from
// either the anthropic-go or openai-go interceptor packages without pulling in
// the other SDK.
//
// Throwaway extraction per AIGOV-532.
package bedrocksig

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"golang.org/x/xerrors"

	aibconfig "github.com/coder/coder/v2/aibridge/config"
)

// SigningService is the AWS SigV4 service name for Bedrock Mantle.
const SigningService = "bedrock-mantle"

// PRMUserAgent is Coder's AWS Partner Revenue Measurement (PRM) attribution
// marker for outbound Bedrock requests. It is appended to the User-Agent
// header so AWS can recognize the traffic as Coder-associated Bedrock usage.
const PRMUserAgent = "sdk-ua-app-id/APN_1.1%2Fpc_cdfmjwn8i6u8l9fwz8h82e4w3%24"

// Runtime carries the Bedrock config and AWS credentials provider shared by
// every interception that targets a Bedrock-backed upstream. It is the
// provider-level handle; per-request state lives on the interceptor.
type Runtime struct {
	Cfg   aibconfig.AWSBedrock
	Creds aws.CredentialsProvider
}

// AppendPRMUserAgent appends the Coder PRM attribution marker to the request's
// User-Agent header.
func AppendPRMUserAgent(req *http.Request) {
	if ua := req.Header.Get("User-Agent"); ua != "" {
		req.Header.Set("User-Agent", ua+" "+PRMUserAgent)
	}
}

// SignMiddleware returns an stdlib HTTP middleware that SigV4-signs the request
// for the Bedrock Mantle service. It appends the PRM user-agent, reads and
// restores the body for hashing, then signs with the given credentials and
// region. The returned function matches the Middleware signature of both the
// anthropic-go and openai-go SDK option packages, so callers pass it directly
// to option.WithMiddleware.
func SignMiddleware(creds aws.CredentialsProvider, region string) func(req *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	signer := v4.NewSigner()
	return func(req *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
		AppendPRMUserAgent(req)

		resolved, err := creds.Retrieve(req.Context())
		if err != nil {
			return nil, xerrors.Errorf("mantle SigV4: resolve AWS credentials: %w", err)
		}

		// SigV4 requires a payload hash, so read the body to hash it and then
		// restore it for the downstream HTTP client to send.
		var body []byte
		if req.Body != nil {
			body, err = io.ReadAll(req.Body)
			if err != nil {
				return nil, xerrors.Errorf("mantle SigV4: read request body: %w", err)
			}
			_ = req.Body.Close()
			req.Body = io.NopCloser(bytes.NewReader(body))
			req.ContentLength = int64(len(body))
		}

		hash := sha256.Sum256(body)
		if err := signer.SignHTTP(req.Context(), resolved, req, hex.EncodeToString(hash[:]), SigningService, region, time.Now()); err != nil {
			return nil, xerrors.Errorf("mantle SigV4: sign request: %w", err)
		}
		return next(req)
	}
}
