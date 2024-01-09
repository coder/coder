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
				"status_code",
				"domain",
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

func (c *Config) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	// No external requests are made when constructing the auth code url.
	return c.underlying.AuthCodeURL(state, opts...)
}

func (c *Config) Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	return c.underlying.Exchange(c.wrapClient(ctx), code, opts...)
}

func (c *Config) TokenSource(ctx context.Context, token *oauth2.Token) oauth2.TokenSource {
	return c.underlying.TokenSource(c.wrapClient(ctx), token)
}

// wrapClient is the only way we can accurately instrument the oauth2 client.
// This is because method calls to the 'OAuth2Config' interface are not 1:1 with
// network requests.
//
// For example, the 'TokenSource' method will return a token
// source that will make a network request when the 'Token' method is called on
// it if the token is expired.
func (c *Config) wrapClient(ctx context.Context) context.Context {
	cli := http.DefaultClient

	// Check if the context has an http client already.
	if hc, ok := ctx.Value(oauth2.HTTPClient).(*http.Client); ok {
		cli = hc
	}

	// The new tripper will instrument every request made by the oauth2 client.
	cli.Transport = newInstrumentedTripper(c, cli.Transport)
	return context.WithValue(ctx, oauth2.HTTPClient, cli)
}

type instrumentedTripper struct {
	c          *Config
	underlying http.RoundTripper
}

func newInstrumentedTripper(c *Config, under http.RoundTripper) *instrumentedTripper {
	if under == nil {
		under = http.DefaultTransport
	}
	return &instrumentedTripper{
		c:          c,
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
		"status_code": fmt.Sprintf("%d", statusCode),
		"domain":      r.URL.Host,
	}).Inc()
	return resp, err
}
