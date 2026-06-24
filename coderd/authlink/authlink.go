package authlink

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// OIDCLinkAnalysis contains the results of analyzing OIDC user links
// grouped by their issuer prefix.
type OIDCLinkAnalysis struct {
	Total            int            // Total OIDC user links
	Unlinked         int            // linked_id == ""
	CorrectIssuer    int            // linked_id starts with expectedIssuer||
	MismatchedCounts map[string]int // issuer -> count for non-matching issuers
}

// MismatchedTotal returns the total number of links with a non-matching issuer.
func (a OIDCLinkAnalysis) MismatchedTotal() int {
	total := 0
	for _, count := range a.MismatchedCounts {
		total += count
	}
	return total
}

// AnalyzeOIDCLinks queries OIDC user links grouped by issuer prefix and
// categorizes them relative to expectedIssuer.
func AnalyzeOIDCLinks(ctx context.Context, db database.Store, expectedIssuer string) (OIDCLinkAnalysis, error) {
	rows, err := db.CountOIDCLinkedIDsByIssuer(ctx)
	if err != nil {
		return OIDCLinkAnalysis{}, xerrors.Errorf("count OIDC linked IDs by issuer: %w", err)
	}

	analysis := OIDCLinkAnalysis{
		MismatchedCounts: make(map[string]int),
	}
	for _, row := range rows {
		count := int(row.Count)
		analysis.Total += count
		switch {
		case row.IssuerPrefix == "":
			analysis.Unlinked += count
		case row.IssuerPrefix == expectedIssuer:
			analysis.CorrectIssuer += count
		default:
			analysis.MismatchedCounts[row.IssuerPrefix] += count
		}
	}
	return analysis, nil
}

// ResetMismatchedOIDCLinks resets linked_id to empty for all OIDC links whose
// issuer prefix does not match expectedIssuer. Returns the number of rows
// affected.
func ResetMismatchedOIDCLinks(ctx context.Context, db database.Store, expectedIssuer string) (int64, error) {
	prefix := expectedIssuer + "||"
	count, err := db.UnlinkOIDCUsersByIssuerMismatch(ctx, prefix)
	if err != nil {
		return 0, xerrors.Errorf("unlink OIDC users by issuer mismatch: %w", err)
	}
	return count, nil
}

// UnmatchableIssuer is a synthetic issuer value that no real OIDC linked_id
// will ever start with. Passing it to AnalyzeOIDCLinks or
// ResetMismatchedOIDCLinks causes every link to be treated as "mismatched",
// which effectively resets all of them.
const UnmatchableIssuer = "00000000-0000-0000-0000-000000000000"

// ResolveIssuer uses OIDC discovery to fetch the canonical issuer string
// from the provider's .well-known/openid-configuration endpoint.
// This does not require OIDC client credentials.
//
// This works the same as `oidc.NewProvider`. The `oidc` package does not
// expose a method to extract the Issuer. So we have to manually make the
// http request.
func ResolveIssuer(ctx context.Context, cli *http.Client, issuerURL string) (string, error) {
	wellKnownURL, err := url.JoinPath(issuerURL, "/.well-known/openid-configuration")
	if err != nil {
		return "", xerrors.Errorf("resolve issuer URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnownURL, nil)
	if err != nil {
		return "", xerrors.Errorf("create discovery request: %w", err)
	}

	resp, err := cli.Do(req)
	if err != nil {
		return "", xerrors.Errorf("fetch OIDC discovery document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", xerrors.Errorf("OIDC discovery returned HTTP %d", resp.StatusCode)
	}

	var discovery struct {
		Issuer string `json:"issuer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		return "", xerrors.Errorf("decode OIDC discovery document: %w", err)
	}
	if discovery.Issuer == "" {
		return "", xerrors.New("OIDC discovery document has empty issuer field")
	}
	return discovery.Issuer, nil
}

// PrintAnalysis writes a human-readable summary of the OIDC link analysis.
// Used for the cli command and debugging.
func PrintAnalysis(w io.Writer, analysis OIDCLinkAnalysis, issuer string) {
	_, _ = fmt.Fprintf(w, "OIDC Link Analysis (issuer: %s)\n", issuer)
	_, _ = fmt.Fprintf(w, "  Total OIDC users:            %d\n", analysis.Total)
	_, _ = fmt.Fprintf(w, "  Correctly linked:            %d\n", analysis.CorrectIssuer)
	_, _ = fmt.Fprintf(w, "  Unlinked (empty linked_id):  %d\n", analysis.Unlinked)

	mismatchedTotal := analysis.MismatchedTotal()
	_, _ = fmt.Fprintf(w, "  Linked to other issuers:     %d\n", mismatchedTotal)

	if mismatchedTotal > 0 {
		// Sort issuer keys for deterministic output.
		issuers := make([]string, 0, len(analysis.MismatchedCounts))
		for issuer := range analysis.MismatchedCounts {
			issuers = append(issuers, issuer)
		}
		sort.Strings(issuers)
		for _, iss := range issuers {
			_, _ = fmt.Fprintf(w, "    %s: %d\n", iss, analysis.MismatchedCounts[iss])
		}
	}
}
