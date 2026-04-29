package provider

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	bedrockintercept "github.com/coder/coder/v2/aibridge/intercept/bedrock"
	"github.com/coder/coder/v2/aibridge/intercept/messages"
	"github.com/coder/coder/v2/aibridge/tracing"
)

var _ Provider = &Bedrock{}

// Bedrock is a standalone Bedrock provider that accepts native Bedrock
// API requests and proxies them to AWS with centralized SigV4 signing.
type Bedrock struct {
	cfg        config.AWSBedrockProvider
	httpClient *http.Client
}

func NewBedrock(cfg config.AWSBedrockProvider) *Bedrock {
	if cfg.Name == "" {
		cfg.Name = config.ProviderBedrock
	}
	if cfg.APIDumpDir == "" {
		cfg.APIDumpDir = os.Getenv("BRIDGE_DUMP_DIR")
	}
	if cfg.CircuitBreaker != nil {
		// TODO: these are Anthropic-specific. Bedrock supports multiple
		// model families with different error formats. Revisit when
		// adding support for non-Anthropic models.
		cfg.CircuitBreaker.IsFailure = anthropicIsFailure
		cfg.CircuitBreaker.OpenErrorResponse = anthropicOpenErrorResponse
	}

	return &Bedrock{
		cfg:        cfg,
		httpClient: &http.Client{},
	}
}

func (*Bedrock) Type() string {
	return config.ProviderBedrock
}

func (p *Bedrock) Name() string {
	return p.cfg.Name
}

func (p *Bedrock) BaseURL() string {
	return p.cfg.AWSBedrock.ResolvedBaseURL()
}

func (p *Bedrock) RoutePrefix() string {
	return fmt.Sprintf("/%s", p.Name())
}

// BridgedRoutes returns a prefix pattern that catches all Bedrock
// invoke paths: /model/{modelId}/invoke and
// /model/{modelId}/invoke-with-response-stream.
func (*Bedrock) BridgedRoutes() []string {
	return []string{"/model/"}
}

// PassthroughRoutes returns an empty slice. All Bedrock requests
// require SigV4 signing which cannot be done via simple header
// injection.
func (*Bedrock) PassthroughRoutes() []string {
	return nil
}

func (*Bedrock) AuthHeader() string {
	return "Authorization"
}

// InjectAuthHeader is a no-op for Bedrock. Authentication is handled
// by SigV4 signing inside the interceptor, not via header injection.
func (*Bedrock) InjectAuthHeader(_ *http.Header) {}

func (p *Bedrock) CircuitBreakerConfig() *config.CircuitBreaker {
	return p.cfg.CircuitBreaker
}

func (p *Bedrock) APIDumpDir() string {
	return p.cfg.APIDumpDir
}

// parseBedrockPath extracts the model ID and streaming flag from a
// Bedrock invoke path. Expected format:
//
//	/model/{modelId}/invoke
//	/model/{modelId}/invoke-with-response-stream
func parseBedrockPath(path string) (modelID string, streaming bool, err error) {
	const modelPrefix = "/model/"
	if !strings.HasPrefix(path, modelPrefix) {
		return "", false, xerrors.Errorf("path does not start with %s: %s", modelPrefix, path)
	}

	rest := path[len(modelPrefix):]

	switch {
	case strings.HasSuffix(rest, "/invoke-with-response-stream"):
		modelID = strings.TrimSuffix(rest, "/invoke-with-response-stream")
		streaming = true
	case strings.HasSuffix(rest, "/invoke"):
		modelID = strings.TrimSuffix(rest, "/invoke")
		streaming = false
	default:
		return "", false, xerrors.Errorf("path does not end with /invoke or /invoke-with-response-stream: %s", path)
	}

	if modelID == "" {
		return "", false, xerrors.Errorf("empty model ID in path: %s", path)
	}

	return modelID, streaming, nil
}

func (p *Bedrock) CreateInterceptor(_ http.ResponseWriter, r *http.Request, tracer trace.Tracer) (_ intercept.Interceptor, outErr error) {
	id := uuid.New()
	_, span := tracer.Start(r.Context(), "Intercept.CreateInterceptor")
	defer tracing.EndSpanErr(span, &outErr)

	path := strings.TrimPrefix(r.URL.Path, p.RoutePrefix())

	modelID, streaming, err := parseBedrockPath(path)
	if err != nil {
		span.SetStatus(codes.Error, "unknown route: "+r.URL.Path)
		return nil, ErrUnknownRoute
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, xerrors.Errorf("read body: %w", err)
	}

	reqPayload, err := messages.NewRequestPayload(body)
	if err != nil {
		return nil, xerrors.Errorf("unmarshal request body: %w", err)
	}

	cred := intercept.NewCredentialInfo(intercept.CredentialKindCentralized, "")

	interceptor := bedrockintercept.NewInterceptor(
		id,
		p.Name(),
		reqPayload,
		p.cfg.AWSBedrock,
		modelID,
		streaming,
		path,
		p.httpClient,
		p.cfg.APIDumpDir,
		tracer,
		cred,
	)

	span.SetAttributes(interceptor.TraceAttributes(r)...)
	return interceptor, nil
}
