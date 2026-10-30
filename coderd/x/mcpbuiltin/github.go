package mcpbuiltin

import (
	"net/http"
	"net/url"
	"strings"
	"sync"

	ghmcp "github.com/github/github-mcp-server/v2/pkg/github"
	ghtransport "github.com/github/github-mcp-server/v2/pkg/http/transport"
	"github.com/github/github-mcp-server/v2/pkg/inventory"
	"github.com/github/github-mcp-server/v2/pkg/observability"
	"github.com/github/github-mcp-server/v2/pkg/observability/metrics"
	"github.com/github/github-mcp-server/v2/pkg/raw"
	"github.com/github/github-mcp-server/v2/pkg/translations"
	ghutils "github.com/github/github-mcp-server/v2/pkg/utils"
	gogithub "github.com/google/go-github/v92/github"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shurcooL/githubv4"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/buildinfo"
)

// GitHub server option query parameters.
const (
	// GitHubOptionAPIBaseURL is the GitHub REST API base URL. Use
	// https://HOST/api/v3 for GitHub Enterprise Server and
	// https://api.SUBDOMAIN.ghe.com for GitHub Enterprise Cloud with
	// data residency.
	GitHubOptionAPIBaseURL = "api_base_url"
	// GitHubOptionToolsets is a comma-separated list of GitHub MCP
	// server toolsets, such as "repos,issues,pull_requests". "default"
	// selects GitHub's default toolsets and "all" selects every toolset.
	GitHubOptionToolsets = "toolsets"
	// GitHubOptionReadOnly hides tools that change anything on GitHub.
	GitHubOptionReadOnly = "read_only"
)

const (
	defaultGitHubAPIBaseURL = "https://api.github.com"
	// githubContentWindowSize matches the GitHub MCP server's default.
	githubContentWindowSize = 5000
)

// githubServer serves the built-in GitHub MCP server by embedding the
// toolsets of GitHub's official MCP server.
type githubServer struct {
	h         *Handler
	exporters observability.Exporters

	mu sync.Mutex
	// catalogs caches the tool, resource, and prompt definitions per
	// host type; building them takes far longer than one request.
	catalogs map[ghutils.HostType]*githubCatalog
}

type githubCatalog struct {
	tools     []inventory.ServerTool
	resources []inventory.ServerResourceTemplate
	prompts   []inventory.ServerPrompt
}

func newGitHubServer(h *Handler) *githubServer {
	exporters, err := observability.NewExporters(h.sdkLogger, metrics.NewNoopMetrics())
	if err != nil {
		// Only fails for nil arguments.
		panic("developer error: create GitHub MCP observability exporters: " + err.Error())
	}
	return &githubServer{
		h:         h,
		exporters: exporters,
		catalogs:  map[ghutils.HostType]*githubCatalog{},
	}
}

func (s *githubServer) catalog(hostType ghutils.HostType) *githubCatalog {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.catalogs[hostType]; ok {
		return c
	}
	t := translations.NullTranslationHelper
	c := &githubCatalog{
		tools:     ghmcp.AllTools(t, ghmcp.WithHost(hostType)),
		resources: ghmcp.AllResources(t),
		prompts:   ghmcp.AllPrompts(t),
	}
	s.catalogs[hostType] = c
	return c
}

// githubEndpoints are the upstream URLs derived from api_base_url.
type githubEndpoints struct {
	hostType ghutils.HostType
	rest     *url.URL
	graphql  *url.URL
	upload   *url.URL
	raw      *url.URL
}

// githubEndpointsFor derives the GraphQL, upload, and raw content URLs
// from a REST API base URL. Unlike the GitHub MCP server's own host
// parsing, it never probes the network.
func githubEndpointsFor(rest *url.URL) (githubEndpoints, error) {
	host := strings.ToLower(rest.Hostname())
	withPath := func(host, p string) *url.URL {
		return &url.URL{Scheme: rest.Scheme, Host: host, Path: p}
	}
	switch {
	case host == "api.github.com":
		if rest.Path != "" {
			return githubEndpoints{}, xerrors.New("api_base_url for github.com must be https://api.github.com")
		}
		return githubEndpoints{
			hostType: ghutils.HostTypeDotcom,
			rest:     withPath(rest.Host, "/"),
			graphql:  withPath(rest.Host, "/graphql"),
			upload:   withPath("uploads.github.com", "/"),
			raw:      withPath("raw.githubusercontent.com", "/"),
		}, nil
	case strings.HasPrefix(host, "api.") && strings.HasSuffix(host, ".ghe.com"):
		if rest.Path != "" {
			return githubEndpoints{}, xerrors.New("api_base_url for GitHub Enterprise Cloud must be https://api.SUBDOMAIN.ghe.com")
		}
		tenant := strings.TrimPrefix(host, "api.")
		return githubEndpoints{
			hostType: ghutils.HostTypeGHEC,
			rest:     withPath(rest.Host, "/"),
			graphql:  withPath(rest.Host, "/graphql"),
			upload:   withPath("uploads."+tenant, "/"),
			raw:      withPath("raw."+tenant, "/"),
		}, nil
	case host == "github.com" || strings.HasSuffix(host, ".github.com") || host == "ghe.com" || strings.HasSuffix(host, ".ghe.com"):
		return githubEndpoints{}, xerrors.Errorf("api_base_url must be the REST API URL, such as %s", defaultGitHubAPIBaseURL)
	default:
		// GitHub Enterprise Server serves every API below one host.
		if rest.Path != "" && rest.Path != "/api/v3" {
			return githubEndpoints{}, xerrors.New("api_base_url for GitHub Enterprise Server must be https://HOST/api/v3")
		}
		return githubEndpoints{
			hostType: ghutils.HostTypeGHES,
			rest:     withPath(rest.Host, "/api/v3/"),
			graphql:  withPath(rest.Host, "/api/graphql"),
			upload:   withPath(rest.Host, "/api/uploads/"),
			raw:      withPath(rest.Host, "/raw/"),
		}, nil
	}
}

func (s *githubServer) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	opts, err := newQueryOptions(r.URL, GitHubOptionAPIBaseURL, GitHubOptionToolsets, GitHubOptionReadOnly)
	if err != nil {
		writeOptionError(ctx, rw, err)
		return
	}
	info := requestInfoFrom(ctx)
	restURL, err := s.h.parseAPIBaseURL(opts.string(GitHubOptionAPIBaseURL), defaultGitHubAPIBaseURL)
	if err != nil {
		writeOptionError(ctx, rw, err)
		return
	}
	endpoints, err := githubEndpointsFor(restURL)
	if err != nil {
		writeOptionError(ctx, rw, err)
		return
	}
	readOnly, err := opts.bool(GitHubOptionReadOnly, false)
	if err != nil {
		writeOptionError(ctx, rw, err)
		return
	}
	toolsets := opts.list(GitHubOptionToolsets)
	if toolsets != nil && len(toolsets) == 0 {
		writeOptionError(ctx, rw, xerrors.New(`toolsets must list at least one toolset; omit it to use GitHub's defaults`))
		return
	}

	catalog := s.catalog(endpoints.hostType)
	inv, err := inventory.NewBuilder().
		SetTools(catalog.tools).
		SetResources(catalog.resources).
		SetPrompts(catalog.prompts).
		WithDeprecatedAliases(ghmcp.DeprecatedToolAliases).
		WithReadOnly(readOnly).
		WithToolsets(toolsets).
		WithServerInstructions().
		Build()
	if err != nil {
		writeOptionError(ctx, rw, xerrors.Errorf("build GitHub toolsets: %w", err))
		return
	}
	if unknown := inv.UnrecognizedToolsets(); len(unknown) > 0 {
		writeOptionError(ctx, rw, xerrors.Errorf("unknown GitHub toolsets: %s", strings.Join(unknown, ", ")))
		return
	}

	deps, err := s.deps(endpoints, info.token)
	if err != nil {
		writeOptionError(ctx, rw, err)
		return
	}
	translator := translations.NullTranslationHelper
	server, err := ghmcp.NewMCPServer(ctx, &ghmcp.MCPServerConfig{
		Version:           buildinfo.Version(),
		Translator:        translator,
		ReadOnly:          readOnly,
		ContentWindowSize: githubContentWindowSize,
		Logger:            s.h.sdkLogger,
		ServerOptions: []ghmcp.MCPServerOption{func(o *mcp.ServerOptions) {
			o.SchemaCache = s.h.schema
		}},
	}, deps, inv)
	if err != nil {
		writeOptionError(ctx, rw, xerrors.Errorf("create GitHub MCP server: %w", err))
		return
	}
	streamableHandler(server).ServeHTTP(rw, r.WithContext(inv.WithFeatureState(ctx)))
}

// deps builds the GitHub API clients for one caller's token. The token is
// only attached to requests bound for the configured GitHub hosts.
func (s *githubServer) deps(endpoints githubEndpoints, token string) (*ghmcp.BaseDeps, error) {
	userAgent := &ghtransport.UserAgentTransport{
		Transport: s.h.upstream.Transport,
		Agent:     "coder-builtin-mcp/" + buildinfo.Version(),
	}
	allowedHosts := []string{endpoints.rest.Host, endpoints.graphql.Host, endpoints.upload.Host, endpoints.raw.Host}
	newClient := func(rt http.RoundTripper) *http.Client {
		return &http.Client{
			Transport: &ghtransport.BearerAuthTransport{
				Transport:    rt,
				Token:        token,
				AllowedHosts: allowedHosts,
			},
			Timeout:       s.h.upstream.Timeout,
			CheckRedirect: s.h.upstream.CheckRedirect,
		}
	}
	httpClient := newClient(userAgent)

	rest, err := gogithub.NewClient(
		gogithub.WithHTTPClient(httpClient),
		gogithub.WithEnterpriseURLs(endpoints.rest.String(), endpoints.upload.String()),
	)
	if err != nil {
		return nil, xerrors.Errorf("create GitHub REST client: %w", err)
	}
	rawClient, err := raw.NewClient(rest, endpoints.raw)
	if err != nil {
		return nil, xerrors.Errorf("create GitHub raw content client: %w", err)
	}
	gql := githubv4.NewEnterpriseClient(endpoints.graphql.String(), newClient(&ghtransport.GraphQLFeaturesTransport{Transport: userAgent}))

	return ghmcp.NewBaseDeps(
		rest,
		gql,
		rawClient,
		nil,
		translations.NullTranslationHelper,
		ghmcp.FeatureFlags{},
		githubContentWindowSize,
		nil,
		s.exporters,
	), nil
}
