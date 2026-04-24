package chatprovider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"golang.org/x/xerrors"
)

var bedrockAnthropicHeaders = []string{
	"Anthropic-Version",
	"X-Api-Key",
}

// bedrockHeaderCleaningHTTPClient removes Anthropic public API headers from
// Bedrock requests. The Anthropic SDK reads ANTHROPIC_API_KEY from the process
// environment and sets X-Api-Key before the Bedrock middleware signs requests.
// Bedrock rejects that mixed header set before returning a normal JSON error.
func bedrockHeaderCleaningHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		return &http.Client{
			Transport: newBedrockHeaderCleaningTransport(http.DefaultTransport),
		}
	}

	clone := *client
	clone.Transport = newBedrockHeaderCleaningTransport(clone.Transport)
	return &clone
}

type bedrockHeaderCleaningTransport struct {
	next http.RoundTripper

	signer *v4.Signer

	configOnce sync.Once
	config     aws.Config
	configErr  error
}

func newBedrockHeaderCleaningTransport(
	next http.RoundTripper,
) *bedrockHeaderCleaningTransport {
	if next == nil {
		next = http.DefaultTransport
	}
	return &bedrockHeaderCleaningTransport{
		next:   next,
		signer: v4.NewSigner(),
	}
}

func (t *bedrockHeaderCleaningTransport) RoundTrip(
	r *http.Request,
) (*http.Response, error) {
	removeBedrockAnthropicHeaders(r.Header)
	if isSigV4Authorization(r.Header.Get("Authorization")) {
		if err := t.resignSigV4(r); err != nil {
			return nil, err
		}
	}
	return t.next.RoundTrip(r)
}

func removeBedrockAnthropicHeaders(headers http.Header) {
	for _, header := range bedrockAnthropicHeaders {
		headers.Del(header)
	}
}

func isSigV4Authorization(authorization string) bool {
	return strings.HasPrefix(authorization, "AWS4-HMAC-SHA256")
}

func (t *bedrockHeaderCleaningTransport) resignSigV4(r *http.Request) error {
	body, err := resetRequestBody(r)
	if err != nil {
		return xerrors.Errorf("read Bedrock request body for signing: %w", err)
	}

	cfg, err := t.awsConfig(r.Context())
	if err != nil {
		return xerrors.Errorf("load AWS config for Bedrock signing: %w", err)
	}
	if cfg.Region == "" {
		return xerrors.New("AWS region is required for Bedrock signing")
	}
	if cfg.Credentials == nil {
		return xerrors.New("AWS credentials are required for Bedrock signing")
	}
	credentials, err := cfg.Credentials.Retrieve(r.Context())
	if err != nil {
		return xerrors.Errorf("retrieve AWS credentials for Bedrock signing: %w", err)
	}

	clearSigV4Headers(r.Header)
	hash := sha256.Sum256(body)
	return t.signer.SignHTTP(
		r.Context(),
		credentials,
		r,
		hex.EncodeToString(hash[:]),
		"bedrock",
		cfg.Region,
		time.Now(),
	)
}

func (t *bedrockHeaderCleaningTransport) awsConfig(
	ctx context.Context,
) (aws.Config, error) {
	t.configOnce.Do(func() {
		t.config, t.configErr = config.LoadDefaultConfig(context.WithoutCancel(ctx))
	})
	return t.config, t.configErr
}

func resetRequestBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if err := r.Body.Close(); err != nil {
		return nil, err
	}

	r.Body = io.NopCloser(bytes.NewReader(body))
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	r.ContentLength = int64(len(body))
	return body, nil
}

func clearSigV4Headers(headers http.Header) {
	headers.Del("Authorization")
	headers.Del("X-Amz-Date")
	headers.Del("X-Amz-Security-Token")
}
