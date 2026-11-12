package codersdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

// PodIdentityOperatorKeyHeader carries the key with which the operator of a
// cluster registers its cluster. The key is good for that and nothing else.
const PodIdentityOperatorKeyHeader = "Coder-Pod-Identity-Operator-Key"

// RegisterPodIdentityIssuerRequest describes a cluster whose pods may be
// workspace daemons.
type RegisterPodIdentityIssuerRequest struct {
	// IssuerURL is the issuer of the cluster's service account tokens.
	IssuerURL string `json:"issuer_url"`
	// JWKS is the cluster's public signing keys, as the cluster publishes
	// them.
	JWKS json.RawMessage `json:"jwks"`
	// Namespaces are the namespaces whose pods may be workspace daemons.
	Namespaces []string `json:"namespaces"`
}

// PodIdentityIssuer is a cluster that coderd believes about its pods.
type PodIdentityIssuer struct {
	Name       string    `json:"name"`
	IssuerURL  string    `json:"issuer_url"`
	KeyIDs     []string  `json:"key_ids"`
	Namespaces []string  `json:"namespaces"`
	SeenAt     time.Time `json:"seen_at" format:"date-time"`
}

// RegisterPodIdentityIssuer registers a cluster, or renews its
// registration. It needs the operator key and no session.
func (c *Client) RegisterPodIdentityIssuer(ctx context.Context, operatorKey, name string, req RegisterPodIdentityIssuerRequest) (PodIdentityIssuer, error) {
	res, err := c.RequestWithoutSessionToken(ctx, http.MethodPut, "/api/v2/podidentity/issuers/"+url.PathEscape(name), req, func(r *http.Request) {
		r.Header.Set(PodIdentityOperatorKeyHeader, operatorKey)
	})
	if err != nil {
		return PodIdentityIssuer{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return PodIdentityIssuer{}, ReadBodyAsError(res)
	}
	var issuer PodIdentityIssuer
	return issuer, ReadBodyAsJSON(res, &issuer)
}

// PodIdentityIssuers lists the clusters whose registration is current.
func (c *Client) PodIdentityIssuers(ctx context.Context) ([]PodIdentityIssuer, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/podidentity/issuers", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var issuers []PodIdentityIssuer
	return issuers, ReadBodyAsJSON(res, &issuers)
}
