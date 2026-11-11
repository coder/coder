// Package workloadstest provides a stand-in for a Kubernetes cluster's
// service account token signer.
package workloadstest

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const (
	// Issuer is the issuer of the tokens a Cluster signs.
	Issuer = "https://kubernetes.default.svc.cluster.local"
	// Audience is the audience of a valid PodToken.
	Audience = "https://coder.example.com"
	// Namespace is the namespace of a valid PodToken.
	Namespace = "workspaces"
	// ServiceAccount is the service account of every PodToken.
	ServiceAccount = "coder-workload"
)

// Cluster signs pod tokens the way a Kubernetes API server does.
type Cluster struct {
	t      testing.TB
	key    *rsa.PrivateKey
	keyID  string
	signer jose.Signer
}

// NewCluster returns a cluster with a fresh signing key.
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
	return &Cluster{t: t, key: key, keyID: keyID, signer: signer}
}

// Keys returns the cluster's public signing keys.
func (c *Cluster) Keys() jose.JSONWebKeySet {
	return jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
		Key: &c.key.PublicKey, KeyID: c.keyID, Algorithm: string(jose.RS256), Use: "sig",
	}}}
}

// JWKS returns the public signing keys as the cluster would publish them.
func (c *Cluster) JWKS() json.RawMessage {
	c.t.Helper()
	raw, err := json.Marshal(c.Keys())
	require.NoError(c.t, err)
	return raw
}

// PodToken describes a projected service account token.
type PodToken struct {
	Issuer    string
	Audience  string
	Namespace string
	PodName   string
	PodUID    string
	Expiry    time.Time
	// NotPodBound omits the pod claim, as in a token requested for a
	// service account alone.
	NotPodBound bool
}

// ValidPodToken returns a token for a pod that expires ten minutes after
// now.
func ValidPodToken(now time.Time, podName, podUID string) PodToken {
	return PodToken{
		Issuer:    Issuer,
		Audience:  Audience,
		Namespace: Namespace,
		PodName:   podName,
		PodUID:    podUID,
		Expiry:    now.Add(10 * time.Minute),
	}
}

// Sign returns the serialized, signed token.
func (c *Cluster) Sign(tok PodToken) string {
	c.t.Helper()
	kubernetes := map[string]any{
		"namespace":      tok.Namespace,
		"serviceaccount": map[string]any{"name": ServiceAccount, "uid": uuid.NewString()},
	}
	if !tok.NotPodBound {
		kubernetes["pod"] = map[string]any{"name": tok.PodName, "uid": tok.PodUID}
	}
	raw, err := jwt.Signed(c.signer).
		Claims(jwt.Claims{
			Issuer:   tok.Issuer,
			Subject:  "system:serviceaccount:" + tok.Namespace + ":" + ServiceAccount,
			Audience: jwt.Audience{tok.Audience},
			Expiry:   jwt.NewNumericDate(tok.Expiry),
			IssuedAt: jwt.NewNumericDate(tok.Expiry.Add(-10 * time.Minute)),
		}).
		Claims(map[string]any{"kubernetes.io": kubernetes}).
		Serialize()
	require.NoError(c.t, err)
	return raw
}
