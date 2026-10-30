package mcpbuiltin

import (
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/safedial"
)

// queryOptions reads the server options carried on the configured URL and
// rejects anything it does not recognize, so a typo fails loudly instead
// of silently using a default.
type queryOptions struct {
	values url.Values
	known  []string
}

func newQueryOptions(u *url.URL, known ...string) (queryOptions, error) {
	values := u.Query()
	for key, vals := range values {
		if !slices.Contains(known, key) {
			return queryOptions{}, xerrors.Errorf("unknown option %q; supported options are: %s", key, strings.Join(known, ", "))
		}
		if len(vals) > 1 {
			return queryOptions{}, xerrors.Errorf("option %q is set more than once", key)
		}
	}
	return queryOptions{values: values, known: known}, nil
}

func (q queryOptions) string(key string) string {
	return strings.TrimSpace(q.values.Get(key))
}

func (q queryOptions) bool(key string, def bool) (bool, error) {
	raw := q.string(key)
	if raw == "" {
		return def, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, xerrors.Errorf("option %q must be true or false", key)
	}
	return v, nil
}

// list parses a comma-separated option. A missing option returns nil so
// callers can tell "unset" from "empty".
func (q queryOptions) list(key string) []string {
	if !q.values.Has(key) {
		return nil
	}
	out := []string{}
	for _, item := range strings.Split(q.values.Get(key), ",") {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" && !slices.Contains(out, item) {
			out = append(out, item)
		}
	}
	return out
}

// parseAPIBaseURL validates an upstream API base URL. The URL must use
// https, carry no credentials, query, or fragment, and must not name a
// blocked IP literal. Hostnames are checked again after DNS resolution by
// the upstream client's guarded dialer.
func (h *Handler) parseAPIBaseURL(raw, def string) (*url.URL, error) {
	if raw == "" {
		raw = def
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, xerrors.Errorf("api_base_url is not a valid URL: %w", err)
	}
	if u.Host == "" || u.Opaque != "" {
		return nil, xerrors.Errorf("api_base_url must be an absolute URL such as %s", def)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, xerrors.New("api_base_url must not contain credentials, a query, or a fragment")
	}
	addr, addrErr := netip.ParseAddr(u.Hostname())
	switch u.Scheme {
	case "https":
	case "http":
		if !h.opts.AllowInsecureHTTP || !isLoopback(u.Hostname()) {
			return nil, xerrors.New("api_base_url must use https")
		}
	default:
		return nil, xerrors.New("api_base_url must use https")
	}
	if addrErr == nil {
		if err := safedial.CheckAddr(addr, safedial.WithAllowedPrefixes(h.opts.AllowedPrefixes...)); err != nil {
			return nil, xerrors.Errorf("api_base_url points to a blocked address: %w", err)
		}
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	u.RawPath = ""
	return u, nil
}

func isLoopback(host string) bool {
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.IsLoopback()
	}
	return strings.EqualFold(host, "localhost")
}
