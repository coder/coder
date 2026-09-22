package cli

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/sync/singleflight"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

// jwtExpirationSkew accounts for clock skew between the client and issuer.
const jwtExpirationSkew = 10 * time.Second

// headerTransport resolves initial headers so command errors surface at startup.
func headerTransport(ctx context.Context, serverURL *url.URL, header []string, headerCommand string) (*codersdk.HeaderTransport, error) {
	var provider codersdk.HeaderProvider
	if headerCommand == "" {
		headers, err := parseHeaders(header)
		if err != nil {
			return nil, err
		}
		provider = codersdk.StaticHeaderProvider{Header: headers}
	} else {
		provider = &commandHeaderProvider{
			ctx:       ctx,
			serverURL: serverURL,
			static:    slices.Clone(header),
			command:   headerCommand,
			clock:     quartz.NewReal(),
		}
		if _, err := provider.Headers(ctx); err != nil {
			return nil, err
		}
	}
	return &codersdk.HeaderTransport{
		Transport: http.DefaultTransport,
		Provider:  provider,
	}, nil
}

type commandHeaderProvider struct {
	ctx       context.Context
	serverURL *url.URL
	static    []string
	command   string
	clock     quartz.Clock

	sf singleflight.Group
	// Cache access is serialized inside sf.Do. Published maps are immutable.
	cached  http.Header
	expires time.Time
}

func (p *commandHeaderProvider) Headers(context.Context) (http.Header, error) {
	value, err, _ := p.sf.Do("headers", func() (any, error) {
		if p.cached != nil && (p.expires.IsZero() || p.clock.Now().Before(p.expires)) {
			return p.cached, nil
		}
		lines, err := runHeaderCommand(p.ctx, p.serverURL, p.command)
		if err != nil {
			return nil, err
		}
		headers, err := parseHeaders(append(slices.Clone(p.static), lines...))
		if err != nil {
			return nil, err
		}
		p.cached, p.expires = headers, headerExpiry(headers)
		return headers, nil
	})
	if err != nil {
		return nil, err
	}
	headers, ok := value.(http.Header)
	if !ok {
		return nil, xerrors.New("unexpected header cache result")
	}
	return headers.Clone(), nil
}

func runHeaderCommand(ctx context.Context, serverURL *url.URL, command string) ([]string, error) {
	shell, caller := "sh", "-c"
	if runtime.GOOS == "windows" {
		shell, caller = "cmd.exe", "/c"
	}
	var stdout bytes.Buffer
	// #nosec G204 - Executing the configured command is the purpose of this option.
	cmd := exec.CommandContext(ctx, shell, caller, command)
	cmd.Env = append(os.Environ(), "CODER_URL="+serverURL.String())
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return nil, xerrors.Errorf("run header command: %w", err)
	}
	var lines []string
	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, xerrors.Errorf("scan header command output: %w", err)
	}
	return lines, nil
}

func parseHeaders(lines []string) (http.Header, error) {
	headers := make(http.Header)
	for i, line := range lines {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, xerrors.Errorf("parse header %d: expected key=value", i+1)
		}
		headers.Add(key, value)
	}
	return headers, nil
}

// headerExpiry uses unverified JWT claims only to schedule refreshes. It does
// not authenticate the headers, which are passed to the server unchanged.
func headerExpiry(headers http.Header) time.Time {
	var earliest time.Time
	for _, values := range headers {
		for _, value := range values {
			if expiry, ok := jwtExpiry(value); ok && (earliest.IsZero() || expiry.Before(earliest)) {
				earliest = expiry
			}
		}
	}
	if earliest.IsZero() {
		return time.Time{}
	}
	return earliest.Add(-jwtExpirationSkew)
}

var headerJWTAlgorithms = []jose.SignatureAlgorithm{
	jose.HS256, jose.HS384, jose.HS512,
	jose.RS256, jose.RS384, jose.RS512,
	jose.ES256, jose.ES384, jose.ES512,
	jose.PS256, jose.PS384, jose.PS512,
	jose.EdDSA,
}

func jwtExpiry(value string) (time.Time, bool) {
	fields := strings.Fields(value)
	switch {
	case len(fields) == 1:
		value = fields[0]
	case len(fields) == 2 && strings.EqualFold(fields[0], "Bearer"):
		value = fields[1]
	default:
		return time.Time{}, false
	}
	token, err := jwt.ParseSigned(value, headerJWTAlgorithms)
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Expiry *jwt.NumericDate `json:"exp"`
	}
	// We don't have the issuer's key. exp is only a refresh hint, not proof
	// of authentication.
	if err := token.UnsafeClaimsWithoutVerification(&claims); err != nil || claims.Expiry == nil {
		return time.Time{}, false
	}
	return claims.Expiry.Time(), true
}
