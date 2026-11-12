// Package podidentityoperator is the go-between of a Kubernetes cluster and
// coderd for pod identity. It runs in the cluster. It reads how the
// cluster's pod tokens can be recognized, which every service account of
// the cluster may read, and registers that with coderd. coderd then needs
// neither a way to the cluster's API nor a credential for it.
package podidentityoperator

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
)

// serviceAccountDir is where a pod finds its service account.
const serviceAccountDir = "/var/run/secrets/kubernetes.io/serviceaccount"

// Options configures an operator.
type Options struct {
	Logger slog.Logger
	// Coder is where the cluster is registered. It needs no session.
	Coder *codersdk.Client
	// OperatorKey is what coderd accepts registrations with.
	OperatorKey string
	// Name is the operator's name for the cluster.
	Name string
	// Namespaces are the namespaces whose pods may be workspace daemons.
	// Empty means the operator's own namespace.
	Namespaces []string
	// Interval is how often the registration is renewed. coderd stops
	// believing a cluster that does not renew.
	Interval time.Duration

	// KubeURL, KubeTokenFile and KubeCAFile say how to reach the cluster's
	// API. Empty means the cluster the operator runs in.
	KubeURL       string
	KubeTokenFile string
	KubeCAFile    string
}

// Operator registers one cluster with coderd.
type Operator struct {
	opts      Options
	kubeURL   string
	tokenFile string
	http      *http.Client
}

// New returns an operator for the cluster the options describe.
func New(opts Options) (*Operator, error) {
	if opts.Coder == nil || opts.OperatorKey == "" || opts.Name == "" {
		return nil, xerrors.New("an operator needs coderd's URL, the operator key and a name for the cluster")
	}
	if opts.Interval <= 0 {
		opts.Interval = 30 * time.Second
	}
	if opts.KubeURL == "" {
		host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
		if host == "" || port == "" {
			return nil, xerrors.New("not running in a cluster: set the Kubernetes API URL")
		}
		opts.KubeURL = "https://" + net.JoinHostPort(host, port)
		if opts.KubeTokenFile == "" {
			opts.KubeTokenFile = serviceAccountDir + "/token"
		}
		if opts.KubeCAFile == "" {
			opts.KubeCAFile = serviceAccountDir + "/ca.crt"
		}
	}
	if len(opts.Namespaces) == 0 {
		own, err := os.ReadFile(serviceAccountDir + "/namespace")
		if err != nil {
			return nil, xerrors.Errorf("name the namespaces whose pods may be workspace daemons: %w", err)
		}
		opts.Namespaces = []string{strings.TrimSpace(string(own))}
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	if opts.KubeCAFile != "" {
		pem, err := os.ReadFile(opts.KubeCAFile)
		if err != nil {
			return nil, xerrors.Errorf("read cluster CA: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, xerrors.New("the cluster CA file holds no certificates")
		}
		transport.TLSClientConfig.RootCAs = pool
	}
	return &Operator{
		opts:      opts,
		kubeURL:   strings.TrimRight(opts.KubeURL, "/"),
		tokenFile: opts.KubeTokenFile,
		http:      &http.Client{Transport: transport, Timeout: 30 * time.Second},
	}, nil
}

// get reads a path of the cluster's API as the operator's service account.
func (o *Operator) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.kubeURL+path, nil)
	if err != nil {
		return nil, err
	}
	if o.tokenFile != "" {
		// The kubelet replaces this token, so it is read every time.
		token, err := os.ReadFile(o.tokenFile)
		if err != nil {
			return nil, xerrors.Errorf("read service account token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	}
	res, err := o.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, xerrors.Errorf("the cluster answered %s with %d: %s", path, res.StatusCode, strings.TrimSpace(string(raw)))
	}
	return raw, nil
}

// Sync reads the cluster's issuer and signing keys and registers them with
// coderd.
func (o *Operator) Sync(ctx context.Context) (codersdk.PodIdentityIssuer, error) {
	raw, err := o.get(ctx, "/.well-known/openid-configuration")
	if err != nil {
		return codersdk.PodIdentityIssuer{}, xerrors.Errorf("read the cluster's issuer: %w", err)
	}
	var discovery struct {
		Issuer string `json:"issuer"`
	}
	if err := json.Unmarshal(raw, &discovery); err != nil || discovery.Issuer == "" {
		return codersdk.PodIdentityIssuer{}, xerrors.New("the cluster did not name the issuer of its tokens")
	}
	// The keys are read from the API the operator already trusts, not
	// from the address the cluster advertises for them.
	jwks, err := o.get(ctx, "/openid/v1/jwks")
	if err != nil {
		return codersdk.PodIdentityIssuer{}, xerrors.Errorf("read the cluster's signing keys: %w", err)
	}
	return o.opts.Coder.RegisterPodIdentityIssuer(ctx, o.opts.OperatorKey, o.opts.Name, codersdk.RegisterPodIdentityIssuerRequest{
		IssuerURL:  discovery.Issuer,
		JWKS:       jwks,
		Namespaces: o.opts.Namespaces,
	})
}

// Run keeps the cluster registered until ctx ends.
func (o *Operator) Run(ctx context.Context) error {
	registered := false
	for {
		issuer, err := o.Sync(ctx)
		switch {
		case err != nil:
			registered = false
			o.opts.Logger.Warn(ctx, "register the cluster with coderd", slog.Error(err))
		case !registered:
			registered = true
			o.opts.Logger.Info(ctx, "registered the cluster with coderd",
				slog.F("name", issuer.Name), slog.F("issuer", issuer.IssuerURL),
				slog.F("key_ids", issuer.KeyIDs), slog.F("namespaces", issuer.Namespaces))
		default:
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(o.opts.Interval):
		}
	}
}
