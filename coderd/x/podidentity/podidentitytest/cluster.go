// Package podidentitytest provides a stand-in for a Kubernetes cluster's
// service account token signer.
package podidentitytest

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/podidentity"
)

const (
	// IssuerURL is the issuer of the tokens a Cluster signs. Real clusters
	// often share this very one.
	IssuerURL = "https://kubernetes.default.svc.cluster.local"
	// Audience is what a valid token is issued for.
	Audience = "https://coder.example.com"
	// Namespace is the namespace of a Cluster's pods.
	Namespace = "workspaces"
	// ServiceAccount is the service account of a Cluster's pods.
	ServiceAccount = "coder-workload"
)

// Pod is a pod of the fake cluster.
type Pod struct {
	Name string
	UID  string
	// Token is the token the kubelet would have obtained for the pod.
	Token string
}

// Cluster signs pod tokens the way a Kubernetes API server does.
type Cluster struct {
	t      testing.TB
	Name   string
	key    *rsa.PrivateKey
	keyID  string
	signer jose.Signer
}

// NewCluster returns a cluster with a signing key of its own.
func NewCluster(t testing.TB) *Cluster {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	keyID := uuid.NewString()
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: keyID}},
		(&jose.SignerOptions{}).WithType("JWT"),
	)
	require.NoError(t, err)
	return &Cluster{t: t, Name: "cluster-" + keyID[:8], key: key, keyID: keyID, signer: signer}
}

// Keys returns the cluster's public signing keys.
func (c *Cluster) Keys() jose.JSONWebKeySet {
	return jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
		Key: &c.key.PublicKey, KeyID: c.keyID, Algorithm: string(jose.RS256), Use: "sig",
	}}}
}

// Issuer is the cluster as its operator registers it.
func (c *Cluster) Issuer(seenAt time.Time) podidentity.Issuer {
	return podidentity.Issuer{
		Name: c.Name, URL: IssuerURL, Keys: c.Keys(), Namespaces: []string{Namespace}, SeenAt: seenAt,
	}
}

// Registry returns a registry that believes this cluster.
func (c *Cluster) Registry() *podidentity.Registry {
	c.t.Helper()
	registry := podidentity.NewRegistry(Audience)
	require.NoError(c.t, registry.Put(c.Issuer(time.Now())))
	return registry
}

// TokenOptions changes what a token says.
type TokenOptions struct {
	Audience  string
	Namespace string
	Expiry    time.Time
	// NotPodBound omits the pod, as in a token made for a service account
	// alone.
	NotPodBound bool
}

// Token signs a token for a pod.
func (c *Cluster) Token(podName, podUID string, opts TokenOptions) string {
	c.t.Helper()
	if opts.Audience == "" {
		opts.Audience = Audience
	}
	if opts.Namespace == "" {
		opts.Namespace = Namespace
	}
	if opts.Expiry.IsZero() {
		opts.Expiry = time.Now().Add(10 * time.Minute)
	}
	kubernetes := map[string]any{
		"namespace":      opts.Namespace,
		"serviceaccount": map[string]any{"name": ServiceAccount, "uid": uuid.NewString()},
	}
	if !opts.NotPodBound {
		kubernetes["pod"] = map[string]any{"name": podName, "uid": podUID}
	}
	raw, err := jwt.Signed(c.signer).Claims(jwt.Claims{
		Issuer:    IssuerURL,
		Subject:   "system:serviceaccount:" + opts.Namespace + ":" + ServiceAccount,
		Audience:  jwt.Audience{opts.Audience},
		Expiry:    jwt.NewNumericDate(opts.Expiry),
		NotBefore: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Hour)),
	}).Claims(map[string]any{"kubernetes.io": kubernetes}).Serialize()
	require.NoError(c.t, err)
	return raw
}

// NewPod adds a pod, with a token of its own.
func (c *Cluster) NewPod(name string) Pod {
	c.t.Helper()
	uid := uuid.NewString()
	return Pod{Name: name, UID: uid, Token: c.Token(name, uid, TokenOptions{})}
}
