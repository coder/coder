package podidentity

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/xerrors"
)

const (
	inClusterTokenFile = "/var/run/secrets/kubernetes.io/serviceaccount/token" //nolint:gosec // A path, not a credential.
	inClusterCAFile    = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
)

// KubeConfig says how to reach a cluster's API server. The zero value is
// the cluster the process runs in.
type KubeConfig struct {
	// URL of the API server.
	URL string
	// TokenFile holds a service account token. It is read on every
	// request, because the kubelet rotates it.
	TokenFile string
	// CAFile holds the certificate authority of the API server.
	CAFile string
}

// KubeClient is a small client for the Kubernetes API. It covers the few
// calls Coder makes, which does not justify a client library dependency.
type KubeClient struct {
	base      *url.URL
	tokenFile string
	http      *http.Client
}

// NewKubeClient returns a client for the cluster in cfg.
func NewKubeClient(cfg KubeConfig) (*KubeClient, error) {
	if cfg.URL == "" {
		host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
		if host == "" || port == "" {
			return nil, xerrors.New("no Kubernetes API URL is set and this process does not run in a cluster")
		}
		cfg.URL = "https://" + host + ":" + port
	}
	if cfg.TokenFile == "" {
		cfg.TokenFile = inClusterTokenFile
	}
	if cfg.CAFile == "" {
		cfg.CAFile = inClusterCAFile
	}
	base, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, xerrors.Errorf("parse Kubernetes API URL: %w", err)
	}
	ca, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, xerrors.Errorf("read Kubernetes CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, xerrors.Errorf("no certificate in %s", cfg.CAFile)
	}
	if _, err := os.Stat(cfg.TokenFile); err != nil {
		return nil, xerrors.Errorf("read Kubernetes token: %w", err)
	}
	return &KubeClient{
		base:      base,
		tokenFile: cfg.TokenFile,
		http: &http.Client{
			Timeout:   30 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}},
		},
	}, nil
}

// StatusError is an error response of the Kubernetes API.
type StatusError struct {
	Code    int
	Message string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("kubernetes API: %d: %s", e.Code, e.Message)
}

// IsNotFound reports whether err says that the object does not exist.
func IsNotFound(err error) bool {
	var status *StatusError
	return xerrors.As(err, &status) && status.Code == http.StatusNotFound
}

// Do sends one request. A nil out discards the response.
func (c *KubeClient) Do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return xerrors.Errorf("marshal request: %w", err)
		}
		body = bytes.NewReader(raw)
	}
	target := *c.base
	target.Path = strings.TrimSuffix(c.base.Path, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return xerrors.Errorf("create request: %w", err)
	}
	token, err := os.ReadFile(c.tokenFile)
	if err != nil {
		return xerrors.Errorf("read Kubernetes token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return xerrors.Errorf("%s %s: %w", method, path, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return xerrors.Errorf("read response: %w", err)
	}
	if res.StatusCode >= http.StatusBadRequest {
		var status struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &status)
		if status.Message == "" {
			status.Message = strings.TrimSpace(string(raw))
		}
		return &StatusError{Code: res.StatusCode, Message: status.Message}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return xerrors.Errorf("decode response of %s %s: %w", method, path, err)
	}
	return nil
}
