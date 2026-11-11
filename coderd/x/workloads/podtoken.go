// Package workloads tracks runtime-provisioned workspace daemons and the
// operators that run them. It is experimental. Operator and pending-request
// state is held in memory; workloads that have a pod are workspace agent rows.
package workloads

import (
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/xerrors"
)

// PodInstanceIDPrefix prefixes the auth instance ID of a workspace agent
// that authenticates as a Kubernetes pod. The pod UID follows.
const PodInstanceIDPrefix = "k8s-pod:"

// podTokenLeeway tolerates clock skew between coderd and a cluster.
const podTokenLeeway = 30 * time.Second

// podTokenAlgorithms are the signature algorithms Kubernetes API servers use
// for service account tokens.
var podTokenAlgorithms = []jose.SignatureAlgorithm{
	jose.RS256, jose.RS384, jose.RS512,
	jose.ES256, jose.ES384, jose.ES512,
	jose.PS256, jose.PS384, jose.PS512,
}

// PodClaims is the identity a cluster asserts in a pod-bound service account
// token.
type PodClaims struct {
	Issuer         string
	Namespace      string
	PodName        string
	PodUID         string
	ServiceAccount string
}

// InstanceID returns the auth instance ID of the agent bound to this pod.
func (c PodClaims) InstanceID() string {
	return PodInstanceIDPrefix + c.PodUID
}

type kubernetesClaims struct {
	Namespace string `json:"namespace"`
	Pod       struct {
		Name string `json:"name"`
		UID  string `json:"uid"`
	} `json:"pod"`
	ServiceAccount struct {
		Name string `json:"name"`
	} `json:"serviceaccount"`
}

type podPrivateClaims struct {
	Kubernetes kubernetesClaims `json:"kubernetes.io"`
}

// LooksLikePodToken reports whether a credential has the shape of a signed
// JWT. Agent tokens are UUIDs and never match.
func LooksLikePodToken(token string) bool {
	return strings.Count(token, ".") == 2
}

func podClaims(std jwt.Claims, private podPrivateClaims) PodClaims {
	return PodClaims{
		Issuer:         std.Issuer,
		Namespace:      private.Kubernetes.Namespace,
		PodName:        private.Kubernetes.Pod.Name,
		PodUID:         private.Kubernetes.Pod.UID,
		ServiceAccount: private.Kubernetes.ServiceAccount.Name,
	}
}

// PeekPodToken returns a token's claims without verifying them. The result
// is only suitable for choosing which keys to verify the token against.
func PeekPodToken(raw string) (PodClaims, error) {
	tok, err := jwt.ParseSigned(raw, podTokenAlgorithms)
	if err != nil {
		return PodClaims{}, xerrors.Errorf("parse pod token: %w", err)
	}
	var (
		std     jwt.Claims
		private podPrivateClaims
	)
	if err := tok.UnsafeClaimsWithoutVerification(&std, &private); err != nil {
		return PodClaims{}, xerrors.Errorf("read pod token claims: %w", err)
	}
	return podClaims(std, private), nil
}

// VerifyPodToken checks a pod-bound service account token against a
// cluster's public keys and returns the pod it identifies. The token must be
// issued by issuer for audience, be unexpired, and name a pod in namespace.
func VerifyPodToken(raw string, keys jose.JSONWebKeySet, issuer, audience, namespace string, now time.Time) (PodClaims, error) {
	tok, err := jwt.ParseSigned(raw, podTokenAlgorithms)
	if err != nil {
		return PodClaims{}, xerrors.Errorf("parse pod token: %w", err)
	}

	candidates := keys.Keys
	if len(tok.Headers) > 0 && tok.Headers[0].KeyID != "" {
		if byID := keys.Key(tok.Headers[0].KeyID); len(byID) > 0 {
			candidates = byID
		}
	}

	var (
		std      jwt.Claims
		private  podPrivateClaims
		verified bool
	)
	for _, key := range candidates {
		if !key.IsPublic() {
			continue
		}
		if err := tok.Claims(key.Key, &std, &private); err == nil {
			verified = true
			break
		}
	}
	if !verified {
		return PodClaims{}, xerrors.New("pod token signature does not match the cluster's keys")
	}

	if std.Expiry == nil {
		return PodClaims{}, xerrors.New("pod token has no expiry")
	}
	if err := std.ValidateWithLeeway(jwt.Expected{
		Issuer:      issuer,
		AnyAudience: jwt.Audience{audience},
		Time:        now,
	}, podTokenLeeway); err != nil {
		return PodClaims{}, xerrors.Errorf("validate pod token: %w", err)
	}

	claims := podClaims(std, private)
	if claims.PodUID == "" || claims.PodName == "" {
		return PodClaims{}, xerrors.New("token is not bound to a pod")
	}
	if claims.Namespace != namespace {
		return PodClaims{}, xerrors.Errorf("pod token is for namespace %q, not %q", claims.Namespace, namespace)
	}
	return claims, nil
}
