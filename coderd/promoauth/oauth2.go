package promoauth

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/oauth2"
)

type Oauth2Source string

const (
	SourceValidateToken    Oauth2Source = "ValidateToken"
	SourceExchange         Oauth2Source = "Exchange"
	SourceTokenSource      Oauth2Source = "TokenSource"
	SourceAppInstallations Oauth2Source = "AppInstallations"
	SourceAuthorizeDevice  Oauth2Source = "AuthorizeDevice"
)

// OAuth2Config exposes a subset of *oauth2.Config functions for easier testing.
// *oauth2.Config should be used instead of implementing this in production.
type OAuth2Config interface {
	AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string
	Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error)
	TokenSource(context.Context, *oauth2.Token) oauth2.TokenSource
}

// InstrumentedOAuth2Config extends OAuth2Config with a `Do` method that allows
// external oauth related calls to be instrumented. This is to support
// "ValidateToken" which is not an oauth2 specified method.
// These calls still count against the api rate limit, and should be instrumented.
type InstrumentedOAuth2Config interface {
	OAuth2Config

	// Do is provided as a convenience method to make a request with the oauth2 client.
	// It mirrors `http.Client.Do`.
	Do(ctx context.Context, source Oauth2Source, req *http.Request) (*http.Response, error)
}

var _ OAuth2Config = (*Config)(nil)

// Factory allows us to have 1 set of metrics for all oauth2 providers.
// Primarily to avoid any prometheus errors registering duplicate metrics.
type Factory struct {
	metrics *metrics
	// optional replace now func
	Now func() time.Time
}

// metrics is the reusable metrics for all oauth2 providers.
type metrics struct {
	externalRequestCount *prometheus.CounterVec

	// if the oauth supports it, rate limit metrics.
	// rateLimit is the defined limit per interval
	rateLimit          *prometheus.GaugeVec
	rateLimitRemaining *prometheus.GaugeVec
	rateLimitUsed      *prometheus.GaugeVec
	// rateLimitReset is unix time of the next interval (when the rate limit resets).
	rateLimitReset *prometheus.GaugeVec
	// rateLimitResetIn is the time in seconds until the rate limit resets.
	// This is included because it is sometimes more helpful to know the limit
	// will reset in 600seconds, rather than at 1704000000 unix time.
	rateLimitResetIn *prometheus.GaugeVec
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
			rateLimit: factory.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "coderd",
				Subsystem: "oauth2",
				Name:      "external_requests_rate_limit_total",
				Help:      "The total number of allowed requests per interval.",
			}, []string{
				"name",
				// Resource allows different rate limits for the same oauth2 provider.
				// Some IDPs have different buckets for different rate limits.
				"resource",
			}),
			rateLimitRemaining: factory.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "coderd",
				Subsystem: "oauth2",
				Name:      "external_requests_rate_limit_remaining",
				Help:      "The remaining number of allowed requests in this interval.",
			}, []string{
				"name",
				"resource",
			}),
			rateLimitUsed: factory.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "coderd",
				Subsystem: "oauth2",
				Name:      "external_requests_rate_limit_used",
				Help:      "The number of requests made in this interval.",
			}, []string{
				"name",
				"resource",
			}),
			rateLimitReset: factory.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "coderd",
				Subsystem: "oauth2",
				Name:      "external_requests_rate_limit_next_reset_unix",
				Help:      "Unix timestamp for when the next interval starts",
			}, []string{
				"name",
				"resource",
			}),
			rateLimitResetIn: factory.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "coderd",
				Subsystem: "oauth2",
				Name:      "external_requests_rate_limit_reset_in_seconds",
				Help:      "Seconds until the next interval",
			}, []string{
				"name",
				"resource",
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

// NewGithub returns a new instrumented oauth2 config for github. It tracks
// rate limits as well as just the external request counts.
//
//nolint:bodyclose
func (f *Factory) NewGithub(name string, under OAuth2Config) *Config {
	cfg := f.New(name, under)
	cfg.interceptors = append(cfg.interceptors, func(resp *http.Response, err error) {
		limits, ok := githubRateLimits(resp, err)
		if !ok {
			return
		}
		labels := prometheus.Labels{
			"name":     cfg.name,
			"resource": limits.Resource,
		}
		// Default to -1 for "do not know"
		resetIn := float64(-1)
		if !limits.Reset.IsZero() {
			now := time.Now()
			if f.Now != nil {
				now = f.Now()
			}
			resetIn = limits.Reset.Sub(now).Seconds()
			if resetIn < 0 {
				// If it just reset, just make it 0.
				resetIn = 0
			}
		}

		f.metrics.rateLimit.With(labels).Set(float64(limits.Limit))
		f.metrics.rateLimitRemaining.With(labels).Set(float64(limits.Remaining))
		f.metrics.rateLimitUsed.With(labels).Set(float64(limits.Used))
		f.metrics.rateLimitReset.With(labels).Set(float64(limits.Reset.Unix()))
		f.metrics.rateLimitResetIn.With(labels).Set(resetIn)
	})
	return cfg
}

type Config struct {
	// Name is a human friendly name to identify the oauth2 provider. This should be
	// deterministic from restart to restart, as it is going to be used as a label in
	// prometheus metrics.
	name       string
	underlying OAuth2Config
	metrics    *metrics
	// interceptors are called after every request made by the oauth2 client.
	interceptors []func(resp *http.Response, err error)
}

func (c *Config) Do(ctx context.Context, source Oauth2Source, req *http.Request) (*http.Response, error) {
	cli := c.oauthHTTPClient(ctx, source)
	return cli.Do(req)
}

func (c *Config) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	// No external requests are made when constructing the auth code url.
	return c.underlying.AuthCodeURL(state, opts...)
}

func (c *Config) Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	return c.underlying.Exchange(c.wrapClient(ctx, SourceExchange), code, opts...)
}

func (c *Config) TokenSource(ctx context.Context, token *oauth2.Token) oauth2.TokenSource {
	return c.underlying.TokenSource(c.wrapClient(ctx, SourceTokenSource), token)
}

// wrapClient is the only way we can accurately instrument the oauth2 client.
// This is because method calls to the 'OAuth2Config' interface are not 1:1 with
// network requests.
//
// For example, the 'TokenSource' method will return a token
// source that will make a network request when the 'Token' method is called on
// it if the token is expired.
func (c *Config) wrapClient(ctx context.Context, source Oauth2Source) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, c.oauthHTTPClient(ctx, source))
}

// oauthHTTPClient returns an http client that will instrument every request made.
func (c *Config) oauthHTTPClient(ctx context.Context, source Oauth2Source) *http.Client {
	cli := &http.Client{}

	// Check if the context has a http client already.
	if hc, ok := ctx.Value(oauth2.HTTPClient).(*http.Client); ok {
		cli = hc
	}

	// The new tripper will instrument every request made by the oauth2 client.
	cli.Transport = newInstrumentedTripper(c, source, cli.Transport)
	return cli
}

type instrumentedTripper struct {
	c          *Config
	source     Oauth2Source
	underlying http.RoundTripper
}

// newInstrumentedTripper intercepts a http request, and increments the
// externalRequestCount metric.
func newInstrumentedTripper(c *Config, source Oauth2Source, under http.RoundTripper) *instrumentedTripper {
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
		"source":      string(i.source),
		"status_code": fmt.Sprintf("%d", statusCode),
	}).Inc()

	// Handle any extra interceptors.
	for _, interceptor := range i.c.interceptors {
		interceptor(resp, err)
	}
	return resp, err
}
