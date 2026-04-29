// Package bedrock provides a SigV4-signing reverse proxy interceptor
// for native Bedrock API requests. It forwards requests to AWS Bedrock
// with centralized AWS credentials and extracts audit metadata from
// the response stream.
package bedrock

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/config"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/intercept/messages"
	"github.com/coder/coder/v2/aibridge/mcp"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/tracing"
)

var _ intercept.Interceptor = &Interceptor{}

// Interceptor is a SigV4-signing reverse proxy for native Bedrock API
// requests. It forwards the request body as-is, signs with centralized
// AWS credentials, and extracts audit metadata from the response.
type Interceptor struct {
	id           uuid.UUID
	providerName string
	reqPayload   messages.RequestPayload

	cfg config.AWSBedrock
	// modelID is the Bedrock model identifier extracted from the URL path.
	modelID string
	// streaming is true for /invoke-with-response-stream, false for /invoke.
	streaming bool
	// upstreamPath is the path forwarded to Bedrock, e.g.
	// /model/us.anthropic.claude-sonnet-4-6/invoke-with-response-stream.
	upstreamPath string
	httpClient   *http.Client

	tracer trace.Tracer
	logger slog.Logger

	recorder   recorder.Recorder
	credential intercept.CredentialInfo
}

func NewInterceptor(
	id uuid.UUID,
	providerName string,
	reqPayload messages.RequestPayload,
	cfg config.AWSBedrock,
	modelID string,
	streaming bool,
	upstreamPath string,
	httpClient *http.Client,
	tracer trace.Tracer,
	cred intercept.CredentialInfo,
) *Interceptor {
	return &Interceptor{
		id:           id,
		providerName: providerName,
		reqPayload:   reqPayload,
		cfg:          cfg,
		modelID:      modelID,
		streaming:    streaming,
		upstreamPath: upstreamPath,
		httpClient:   httpClient,
		tracer:       tracer,
		credential:   cred,
	}
}

func (i *Interceptor) ID() uuid.UUID { return i.id }
func (i *Interceptor) Model() string { return i.modelID }

func (i *Interceptor) Setup(logger slog.Logger, rec recorder.Recorder, _ mcp.ServerProxier) {
	i.logger = logger
	i.recorder = rec
}

func (i *Interceptor) Streaming() bool                      { return i.streaming }
func (i *Interceptor) Credential() intercept.CredentialInfo { return i.credential }
func (i *Interceptor) CorrelatingToolCallID() *string       { return i.reqPayload.CorrelatingToolCallID() }

func (i *Interceptor) TraceAttributes(r *http.Request) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(tracing.RequestPath, r.URL.Path),
		attribute.String(tracing.InterceptionID, i.id.String()),
		attribute.String(tracing.InitiatorID, aibcontext.ActorIDFromContext(r.Context())),
		attribute.String(tracing.Provider, i.providerName),
		attribute.String(tracing.Model, i.modelID),
		attribute.Bool(tracing.Streaming, i.streaming),
		attribute.Bool(tracing.IsBedrock, true),
	}
}

func (i *Interceptor) ProcessRequest(w http.ResponseWriter, r *http.Request) (outErr error) {
	if len(i.reqPayload) == 0 {
		return xerrors.New("developer error: request payload is empty")
	}

	ctx, span := i.tracer.Start(r.Context(), "Intercept.ProcessRequest", trace.WithAttributes(tracing.InterceptionAttributesFromContext(r.Context())...))
	defer tracing.EndSpanErr(span, &outErr)

	outReq, err := http.NewRequestWithContext(ctx, http.MethodPost, i.cfg.ResolvedBaseURL()+i.upstreamPath, bytes.NewReader(i.reqPayload))
	if err != nil {
		return xerrors.Errorf("create outbound request: %w", err)
	}

	outReq.Header = intercept.PrepareClientHeaders(r.Header)

	awsCreds, err := i.loadCredentials(ctx)
	if err != nil {
		return xerrors.Errorf("load AWS credentials: %w", err)
	}

	hash := sha256.Sum256(i.reqPayload)
	signer := v4.NewSigner()
	if err = signer.SignHTTP(ctx, awsCreds, outReq, hex.EncodeToString(hash[:]), "bedrock", i.cfg.Region, time.Now()); err != nil {
		return xerrors.Errorf("sign request: %w", err)
	}

	resp, err := i.httpClient.Do(outReq)
	if err != nil {
		return xerrors.Errorf("send request to bedrock: %w", err)
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, val := range values {
			w.Header().Add(key, val)
		}
	}
	w.WriteHeader(resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		if _, err = io.Copy(w, resp.Body); err != nil {
			return xerrors.Errorf("write error response body: %w", err)
		}
		return xerrors.Errorf("bedrock returned %s", resp.Status)
	}

	if err = copyResponse(w, resp.Body, i.streaming); err != nil {
		return err
	}

	return nil
}

// copyResponse pipes src to w. When streaming is true, each chunk is
// flushed immediately so the client receives data without delay.
func copyResponse(w http.ResponseWriter, src io.Reader, streaming bool) error {
	if !streaming {
		if _, err := io.Copy(w, src); err != nil {
			return xerrors.Errorf("copy response body: %w", err)
		}
		return nil
	}

	flusher, ok := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return xerrors.Errorf("write streaming chunk: %w", writeErr)
			}
			if ok {
				flusher.Flush()
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return nil
			}
			return xerrors.Errorf("read streaming chunk: %w", readErr)
		}
	}
}

func (i *Interceptor) loadCredentials(ctx context.Context) (aws.Credentials, error) {
	loadOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(i.cfg.Region),
	}

	switch {
	case i.cfg.AccessKey != "" && i.cfg.AccessKeySecret != "":
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				i.cfg.AccessKey,
				i.cfg.AccessKeySecret,
				"",
			),
		))
	case i.cfg.AccessKey != "" || i.cfg.AccessKeySecret != "":
		return aws.Credentials{}, xerrors.New("both access key and access key secret must be provided together")
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return aws.Credentials{}, xerrors.Errorf("load AWS config: %w", err)
	}

	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return aws.Credentials{}, xerrors.Errorf("retrieve AWS credentials: %w", err)
	}

	return creds, nil
}
