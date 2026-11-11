package workloadoperator

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/xerrors"
)

const serviceAccountDir = "/var/run/secrets/kubernetes.io/serviceaccount"

// KubeClient is a minimal Kubernetes API client. The operator needs a
// handful of calls, which does not justify a client library dependency.
type KubeClient struct {
	baseURL    string
	tokenFile  string
	httpClient *http.Client
}

// KubeConfig locates a Kubernetes API server. Empty fields fall back to the
// in-cluster service account.
type KubeConfig struct {
	URL       string
	TokenFile string
	CAFile    string
}

// NewKubeClient returns a client for the configured API server.
func NewKubeClient(cfg KubeConfig) (*KubeClient, error) {
	if cfg.URL == "" {
		host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
		if host == "" || port == "" {
			return nil, xerrors.New("not running in a cluster: set the Kubernetes API URL")
		}
		cfg.URL = "https://" + net.JoinHostPort(host, port)
		if cfg.TokenFile == "" {
			cfg.TokenFile = serviceAccountDir + "/token"
		}
		if cfg.CAFile == "" {
			cfg.CAFile = serviceAccountDir + "/ca.crt"
		}
	}

	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	if cfg.CAFile != "" {
		pem, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, xerrors.Errorf("read cluster CA: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, xerrors.New("cluster CA file holds no certificates")
		}
		transport.TLSClientConfig.RootCAs = pool
	}
	return &KubeClient{
		baseURL:    strings.TrimRight(cfg.URL, "/"),
		tokenFile:  cfg.TokenFile,
		httpClient: &http.Client{Transport: transport, Timeout: 30 * time.Second},
	}, nil
}

// KubeError is a non-success response from the Kubernetes API.
type KubeError struct {
	Status int
	Body   string
}

func (e *KubeError) Error() string {
	return fmt.Sprintf("kubernetes API returned %d: %s", e.Status, e.Body)
}

// do sends a request and returns the raw response body.
func (k *KubeClient) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, xerrors.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, k.baseURL+path, reader)
	if err != nil {
		return nil, xerrors.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if k.tokenFile != "" {
		// The kubelet rotates this token, so it is read for every request.
		token, err := os.ReadFile(k.tokenFile)
		if err != nil {
			return nil, xerrors.Errorf("read service account token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	}
	res, err := k.httpClient.Do(req)
	if err != nil {
		return nil, xerrors.Errorf("%s %s: %w", method, path, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return nil, xerrors.Errorf("read response: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, &KubeError{Status: res.StatusCode, Body: string(raw)}
	}
	return raw, nil
}

func (k *KubeClient) list(ctx context.Context, path, selector string) ([]object, error) {
	raw, err := k.do(ctx, http.MethodGet, path+"?labelSelector="+url.QueryEscape(selector), nil)
	if err != nil {
		return nil, err
	}
	var list struct {
		Items []object `json:"items"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, xerrors.Errorf("decode list: %w", err)
	}
	return list.Items, nil
}

// Deployments lists the Deployments in a namespace that match a selector.
func (k *KubeClient) Deployments(ctx context.Context, namespace, selector string) ([]object, error) {
	return k.list(ctx, "/apis/apps/v1/namespaces/"+namespace+"/deployments", selector)
}

// Pods lists the pods in a namespace that match a selector.
func (k *KubeClient) Pods(ctx context.Context, namespace, selector string) ([]object, error) {
	return k.list(ctx, "/api/v1/namespaces/"+namespace+"/pods", selector)
}

// CreatePod creates a pod and returns it as the API server stored it.
func (k *KubeClient) CreatePod(ctx context.Context, namespace string, pod object) (object, error) {
	raw, err := k.do(ctx, http.MethodPost, "/api/v1/namespaces/"+namespace+"/pods", pod)
	if err != nil {
		return nil, err
	}
	var created object
	return created, json.Unmarshal(raw, &created)
}

// DeletePod deletes a pod. A pod that is already gone is not an error.
func (k *KubeClient) DeletePod(ctx context.Context, namespace, name string) error {
	_, err := k.do(ctx, http.MethodDelete, "/api/v1/namespaces/"+namespace+"/pods/"+name, nil)
	var kubeErr *KubeError
	if xerrors.As(err, &kubeErr) && kubeErr.Status == http.StatusNotFound {
		return nil
	}
	return err
}

// TokenKeys returns the issuer and public signing keys of the cluster's
// service account tokens. They let coderd verify pod tokens offline.
func (k *KubeClient) TokenKeys(ctx context.Context) (issuer string, jwks json.RawMessage, err error) {
	raw, err := k.do(ctx, http.MethodGet, "/.well-known/openid-configuration", nil)
	if err != nil {
		return "", nil, xerrors.Errorf("get token issuer: %w", err)
	}
	var discovery struct {
		Issuer string `json:"issuer"`
	}
	if err := json.Unmarshal(raw, &discovery); err != nil || discovery.Issuer == "" {
		return "", nil, xerrors.New("cluster did not report a token issuer")
	}
	jwks, err = k.do(ctx, http.MethodGet, "/openid/v1/jwks", nil)
	if err != nil {
		return "", nil, xerrors.Errorf("get token signing keys: %w", err)
	}
	return discovery.Issuer, jwks, nil
}
