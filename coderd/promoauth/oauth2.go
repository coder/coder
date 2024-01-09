package promoauth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/oauth2"
)

// OAuth2Config exposes a subset of *oauth2.Config functions for easier testing.
// *oauth2.Config should be used instead of implementing this in production.
type OAuth2Config interface {
	AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string
	Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error)
	TokenSource(context.Context, *oauth2.Token) oauth2.TokenSource
}

type InstrumentedeOAuth2Config interface {
	OAuth2Config

	// Do is provided as a convience method to make a request with the oauth2 client.
	// It mirrors `http.Client.Do`.
	// We need this because Coder adds some extra functionality to
	// oauth clients such as the `ValidateToken()` method.
	Do(ctx context.Context, source string, req *http.Request) (*http.Response, error)
}

type HTTPDo interface {
	// Do is provided as a convience method to make a request with the oauth2 client.
	// It mirrors `http.Client.Do`.
	// We need this because Coder adds some extra functionality to
	// oauth clients such as the `ValidateToken()` method.
	Do(ctx context.Context, source string, req *http.Request) (*http.Response, error)
}

var _ OAuth2Config = (*Config)(nil)

type Factory struct {
	metrics *metrics
}

// metrics is the reusable metrics for all oauth2 providers.
type metrics struct {
	externalRequestCount *prometheus.CounterVec
}

func NewFactory(registry prometheus.Registerer) *Factory {
	factory := promauto.With(registry)

	return &Factory{
		metrics: &metrics{
			externalRequestCount: factory.NewCounterVec(prometheus.CounterOpts{
				Namespace: "coderd",
				Subsystem: "oauth2",
				Name:      "external_requests_total",
				Help:      "The total number of api calls made to external oauth2 providers. 'status_code' will be 0 if the request failed with no response.",
			}, []string{
				"name",
				"source",
				"status_code",
			}),
		},
	}
}

func (f *Factory) New(name string, under OAuth2Config) *Config {
	return &Config{
		name:       name,
		underlying: under,
		metrics:    f.metrics,
	}
}

type Config struct {
	// Name is a human friendly name to identify the oauth2 provider. This should be
	// deterministic from restart to restart, as it is going to be used as a label in
	// prometheus metrics.
	name       string
	underlying OAuth2Config
	metrics    *metrics
}

func (c *Config) Do(ctx context.Context, source string, req *http.Request) (*http.Response, error) {
	cli := c.oauthHTTPClient(ctx, source)
	return cli.Do(req)
}

func (c *Config) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	// No external requests are made when constructing the auth code url.
	return c.underlying.AuthCodeURL(state, opts...)
}

func (c *Config) Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	return c.underlying.Exchange(c.wrapClient(ctx, "Exchange"), code, opts...)
}

func (c *Config) TokenSource(ctx context.Context, token *oauth2.Token) oauth2.TokenSource {
	return c.underlying.TokenSource(c.wrapClient(ctx, "TokenSource"), token)
}

// wrapClient is the only way we can accurately instrument the oauth2 client.
// This is because method calls to the 'OAuth2Config' interface are not 1:1 with
// network requests.
//
// For example, the 'TokenSource' method will return a token
// source that will make a network request when the 'Token' method is called on
// it if the token is expired.
func (c *Config) wrapClient(ctx context.Context, source string) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, c.oauthHTTPClient(ctx, source))
}

func (c *Config) oauthHTTPClient(ctx context.Context, source string) *http.Client {
	cli := &http.Client{}

	// Check if the context has an http client already.
	if hc, ok := ctx.Value(oauth2.HTTPClient).(*http.Client); ok {
		cli = hc
	}

	// The new tripper will instrument every request made by the oauth2 client.
	cli.Transport = newInstrumentedTripper(c, source, cli.Transport)
	return cli
}

type instrumentedTripper struct {
	c          *Config
	source     string
	underlying http.RoundTripper
}

func newInstrumentedTripper(c *Config, source string, under http.RoundTripper) *instrumentedTripper {
	if under == nil {
		under = http.DefaultTransport
	}

	// If the underlying transport is the default, we need to clone it.
	// We should also clone it if it supports cloning.
	if tr, ok := under.(*http.Transport); ok {
		under = tr.Clone()
	}

	return &instrumentedTripper{
		c:          c,
		source:     source,
		underlying: under,
	}
}

func (i *instrumentedTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := i.underlying.RoundTrip(r)
	var statusCode int
	if resp != nil {
		statusCode = resp.StatusCode
	}
	i.c.metrics.externalRequestCount.With(prometheus.Labels{
		"name":        i.c.name,
		"source":      i.source,
		"status_code": fmt.Sprintf("%d", statusCode),
	}).Inc()

	return resp, err
}
