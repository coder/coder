package site

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	htmltemplate "html/template"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"text/template" // html/template escapes some nonces
	"time"

	"github.com/google/uuid"
	"github.com/justinas/nosurf"
	"github.com/unrolled/secure"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/appearance"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/entitlements"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/codersdk"
)

// We always embed the error page HTML because it it doesn't need to be built,
// and it's tiny and doesn't contribute much to the binary size.
var (
	//go:embed static/error.html
	errorHTML string

	errorTemplate *htmltemplate.Template

	//go:embed static/oauth2allow.html
	oauthHTML string

	oauthTemplate *htmltemplate.Template
)

func init() {
	var err error
	errorTemplate, err = htmltemplate.New("error").Parse(errorHTML)
	if err != nil {
		panic(err)
	}

	oauthTemplate, err = htmltemplate.New("error").Parse(oauthHTML)
	if err != nil {
		panic(err)
	}
}

type Options struct {
	BinFS             http.FileSystem
	BinHashes         map[string]string
	Database          database.Store
	SiteFS            fs.FS
	OAuth2Configs     *httpmw.OAuth2Configs
	DocsURL           string
	BuildInfo         codersdk.BuildInfoResponse
	AppearanceFetcher *atomic.Pointer[appearance.Fetcher]
	Entitlements      *entitlements.Set
	Telemetry         telemetry.Reporter
	Logger            slog.Logger
	HideAITasks       bool
}

func New(opts *Options) *Handler {
	if opts.AppearanceFetcher == nil {
		daf := atomic.Pointer[appearance.Fetcher]{}
		f := appearance.NewDefaultFetcher(opts.DocsURL)
		daf.Store(&f)
		opts.AppearanceFetcher = &daf
	}
	handler := &Handler{
		opts:          opts,
		secureHeaders: secureHeaders(),
		Entitlements:  opts.Entitlements,
	}

	// html files are handled by a text/template. Non-html files
	// are served by the default file server.
	var err error
	handler.htmlTemplates, err = findAndParseHTMLFiles(opts.SiteFS)
	if err != nil {
		panic(fmt.Sprintf("Failed to parse html files: %v", err))
	}

	mux := http.NewServeMux()
	mux.Handle("/bin/", newBinHandler(opts))
	mux.Handle("/", http.FileServer(
		http.FS(
			// OnlyFiles is a wrapper around the file system that prevents directory
			// listings. Directory listings are not required for the site file system, so we
			// exclude it as a security measure. In practice, this file system comes from our
			// open source code base, but this is considered a best practice for serving
			// static files.
			OnlyFiles(opts.SiteFS))),
	)
	buildInfoResponse, err := json.Marshal(opts.BuildInfo)
	if err != nil {
		panic("failed to marshal build info: " + err.Error())
	}
	handler.buildInfoJSON = html.EscapeString(string(buildInfoResponse))
	handler.handler = mux.ServeHTTP

	handler.installScript, err = parseInstallScript(opts.SiteFS, opts.BuildInfo, "install.sh")
	if err != nil {
		opts.Logger.Warn(context.Background(), "could not parse install.sh, it will be unavailable", slog.Error(err))
	}

	handler.installScriptWindows, err = parseInstallScript(opts.SiteFS, opts.BuildInfo, "install.ps1")
	if err != nil {
		opts.Logger.Warn(context.Background(), "could not parse install.ps1, it will be unavailable", slog.Error(err))
	}

	return handler
}

type Handler struct {
	opts *Options

	secureHeaders        *secure.Secure
	handler              http.HandlerFunc
	htmlTemplates        *template.Template
	buildInfoJSON        string
	installScript        []byte
	installScriptWindows []byte

	// RegionsFetcher will attempt to fetch the more detailed WorkspaceProxy data, but will fall back to the
	// regions if the user does not have the correct permissions.
	RegionsFetcher func(ctx context.Context) (any, error)

	Entitlements *entitlements.Set
	Experiments  atomic.Pointer[codersdk.Experiments]

	telemetryHTMLServedOnce sync.Once
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	err := h.secureHeaders.Process(rw, r)
	if err != nil {
		return
	}

	// reqFile is the static file requested
	reqFile := filePath(r.URL.Path)
	state := htmlState{
		// Token is the CSRF token for the given request
		CSRF:      csrfState{Token: nosurf.Token(r)},
		BuildInfo: h.buildInfoJSON,
		DocsURL:   h.opts.DocsURL,
	}

	// First check if it's a file we have in our templates
	if h.serveHTML(rw, r, reqFile, state) {
		return
	}

	switch {
	// If requesting binaries, serve straight up.
	case reqFile == "bin" || strings.HasPrefix(reqFile, "bin/"):
		h.handler.ServeHTTP(rw, r)
		return
	// If requesting assets, serve straight up with caching.
	case reqFile == "assets" || strings.HasPrefix(reqFile, "assets/") || strings.HasPrefix(reqFile, "icon/"):
		// It could make sense to cache 404s, but the problem is that during an
		// upgrade a load balancer may route partially to the old server, and that
		// would make new asset paths get cached as 404s and not load even once the
		// new server was in place.  To combat that, only cache if we have the file.
		if h.exists(reqFile) && ShouldCacheFile(reqFile) {
			rw.Header().Add("Cache-Control", "public, max-age=31536000, immutable")
		}
		// If the asset does not exist, this will return a 404.
		h.handler.ServeHTTP(rw, r)
		return
	// If requesting the install.sh script, respond with the preprocessed version
	// which contains the correct hostname and version information.
	case reqFile == "install.sh":
		if h.installScript == nil {
			http.NotFound(rw, r)
			return
		}
		rw.Header().Add("Content-Type", "text/plain; charset=utf-8")
		http.ServeContent(rw, r, reqFile, time.Time{}, bytes.NewReader(h.installScript))
		return
	// If requesting the install.ps1 script (Windows), respond with the preprocessed version
	// which contains the correct hostname and version information.
	case reqFile == "install.ps1":
		if h.installScriptWindows == nil {
			http.NotFound(rw, r)
			return
		}
		rw.Header().Add("Content-Type", "text/plain; charset=utf-8")
		http.ServeContent(rw, r, reqFile, time.Time{}, bytes.NewReader(h.installScriptWindows))
		return
	// If the original file path exists we serve it.
	case h.exists(reqFile):
		if ShouldCacheFile(reqFile) {
			rw.Header().Add("Cache-Control", "public, max-age=31536000, immutable")
		}
		h.handler.ServeHTTP(rw, r)
		return
	}

	// Serve the file assuming it's an html file
	// This matches paths like `/app/terminal.html`
	r.URL.Path = strings.TrimSuffix(r.URL.Path, "/")
	r.URL.Path += ".html"

	reqFile = filePath(r.URL.Path)
	// All html files should be served by the htmlFile templates
	if h.serveHTML(rw, r, reqFile, state) {
		return
	}

	// If we don't have the file... we should redirect to `/`
	// for our single-page-app.
	r.URL.Path = "/"
	if h.serveHTML(rw, r, "", state) {
		return
	}

	// This will send a correct 404
	h.handler.ServeHTTP(rw, r)
}

// filePath returns the filepath of the requested file.
func filePath(p string) string {
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimPrefix(path.Clean(p), "/")
}

func (h *Handler) exists(filePath string) bool {
	f, err := h.opts.SiteFS.Open(filePath)
	if err == nil {
		_ = f.Close()
	}
	return err == nil
}

type htmlState struct {
	CSRF csrfState

	// Below are HTML escaped JSON strings of the respective structs.
	ApplicationName string
	LogoURL         string

	BuildInfo      string
	User           string
	Entitlements   string
	Appearance     string
	UserAppearance string
	Experiments    string
	Regions        string
	DocsURL        string

	TasksTabVisible string
}

type csrfState struct {
	Token string
}

func ShouldCacheFile(reqFile string) bool {
	// Images, favicons and uniquely content hashed bundle assets should be
	// cached. By default, we cache everything in the site/out directory except
	// for deny-listed items enumerated here. The reason for this approach is that
	// cache invalidation techniques should be used by default for all build
	// processed assets. The scenarios where we don't use cache invalidation
	// techniques are one-offs or things that should have invalidation in the
	// future.
	denyListedSuffixes := []string{
		".html",
		"worker.js",
	}

	for _, suffix := range denyListedSuffixes {
		if strings.HasSuffix(reqFile, suffix) {
			return false
		}
	}

	return true
}

// reportHTMLFirstServedAt sends a telemetry report when the first HTML is ever served.
// The purpose is to track the first time the first user opens the site.
func (h *Handler) reportHTMLFirstServedAt() {
	// nolint:gocritic // Manipulating telemetry items is system-restricted.
	// TODO(hugodutka): Add a telemetry context in RBAC.
	ctx := dbauthz.AsSystemRestricted(context.Background())
	itemKey := string(telemetry.TelemetryItemKeyHTMLFirstServedAt)
	_, err := h.opts.Database.GetTelemetryItem(ctx, itemKey)
	if err == nil {
		// If the value is already set, then we reported it before.
		// We don't need to report it again.
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		h.opts.Logger.Debug(ctx, "failed to get telemetry html first served at", slog.Error(err))
		return
	}
	if err := h.opts.Database.InsertTelemetryItemIfNotExists(ctx, database.InsertTelemetryItemIfNotExistsParams{
		Key:   string(telemetry.TelemetryItemKeyHTMLFirstServedAt),
		Value: time.Now().Format(time.RFC3339),
	}); err != nil {
		h.opts.Logger.Debug(ctx, "failed to set telemetry html first served at", slog.Error(err))
		return
	}
	item, err := h.opts.Database.GetTelemetryItem(ctx, itemKey)
	if err != nil {
		h.opts.Logger.Debug(ctx, "failed to get telemetry html first served at", slog.Error(err))
		return
	}
	h.opts.Telemetry.Report(&telemetry.Snapshot{
		TelemetryItems: []telemetry.TelemetryItem{telemetry.ConvertTelemetryItem(item)},
	})
}

func (h *Handler) serveHTML(resp http.ResponseWriter, request *http.Request, reqPath string, state htmlState) bool {
	if data, err := h.renderHTMLWithState(request, reqPath, state); err == nil {
		if reqPath == "" {
			// Pass "index.html" to the ServeContent so the ServeContent sets the right content headers.
			reqPath = "index.html"
		}
		// `Once` is used to reduce the volume of db calls and telemetry reports.
		// It's fine to run the enclosed function multiple times, but it's unnecessary.
		h.telemetryHTMLServedOnce.Do(func() {
			go h.reportHTMLFirstServedAt()
		})
		http.ServeContent(resp, request, reqPath, time.Time{}, bytes.NewReader(data))
		return true
	}
	return false
}

func execTmpl(tmpl *template.Template, state htmlState) ([]byte, error) {
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, state)
	return buf.Bytes(), err
}

// renderWithState will render the file using the given nonce if the file exists
// as a template. If it does not, it will return an error.
func (h *Handler) renderHTMLWithState(r *http.Request, filePath string, state htmlState) ([]byte, error) {
	af := *(h.opts.AppearanceFetcher.Load())
	if filePath == "" {
		filePath = "index.html"
	}
	tmpl := h.htmlTemplates.Lookup(filePath)
	if tmpl == nil {
		return nil, xerrors.Errorf("template %q not found", filePath)
	}

	// Cookies are sent when requesting HTML, so we can get the user
	// and pre-populate the state for the frontend to reduce requests.
	// We use a noop response writer because we don't want to write
	// anything to the response and break the HTML, an error means we
	// simply don't pre-populate the state.
	noopRW := noopResponseWriter{}
	apiKey, actor, ok := httpmw.ExtractAPIKey(noopRW, r, httpmw.ExtractAPIKeyConfig{
		Optional:      true,
		DB:            h.opts.Database,
		OAuth2Configs: h.opts.OAuth2Configs,
		// Special case for site, we can always disable refresh here because
		// the frontend will perform API requests if this fails.
		DisableSessionExpiryRefresh: true,
		RedirectToLogin:             false,
		SessionTokenFunc:            nil,
	})
	if !ok || apiKey == nil || actor == nil {
		var cfg codersdk.AppearanceConfig
		// nolint:gocritic // User is not expected to be signed in.
		ctx := dbauthz.AsSystemRestricted(r.Context())
		cfg, _ = af.Fetch(ctx)
		state.ApplicationName = applicationNameOrDefault(cfg)
		state.LogoURL = cfg.LogoURL
		return execTmpl(tmpl, state)
	}

	ctx := dbauthz.As(r.Context(), *actor)

	var eg errgroup.Group
	var user database.User
	var themePreference string
	var terminalFont string
	orgIDs := []uuid.UUID{}
	eg.Go(func() error {
		var err error
		user, err = h.opts.Database.GetUserByID(ctx, apiKey.UserID)
		return err
	})
	eg.Go(func() error {
		var err error
		themePreference, err = h.opts.Database.GetUserThemePreference(ctx, apiKey.UserID)
		if errors.Is(err, sql.ErrNoRows) {
			themePreference = ""
			return nil
		}
		return err
	})
	eg.Go(func() error {
		var err error
		terminalFont, err = h.opts.Database.GetUserTerminalFont(ctx, apiKey.UserID)
		if errors.Is(err, sql.ErrNoRows) {
			terminalFont = ""
			return nil
		}
		return err
	})
	eg.Go(func() error {
		memberIDs, err := h.opts.Database.GetOrganizationIDsByMemberIDs(ctx, []uuid.UUID{apiKey.UserID})
		if errors.Is(err, sql.ErrNoRows) || len(memberIDs) == 0 {
			return nil
		}
		if err != nil {
			return nil
		}
		orgIDs = memberIDs[0].OrganizationIDs
		return err
	})
	err := eg.Wait()
	if err == nil {
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			user, err := json.Marshal(db2sdk.User(user, orgIDs))
			if err == nil {
				state.User = html.EscapeString(string(user))
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			userAppearance, err := json.Marshal(codersdk.UserAppearanceSettings{
				ThemePreference: themePreference,
				TerminalFont:    codersdk.TerminalFontName(terminalFont),
			})
			if err == nil {
				state.UserAppearance = html.EscapeString(string(userAppearance))
			}
		}()

		if h.Entitlements != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				state.Entitlements = html.EscapeString(string(h.Entitlements.AsJSON()))
			}()
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg, err := af.Fetch(ctx)
			if err == nil {
				appr, err := json.Marshal(cfg)
				if err == nil {
					state.Appearance = html.EscapeString(string(appr))
					state.ApplicationName = applicationNameOrDefault(cfg)
					state.LogoURL = cfg.LogoURL
				}
			}
		}()

		if h.RegionsFetcher != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				regions, err := h.RegionsFetcher(ctx)
				if err == nil {
					regions, err := json.Marshal(regions)
					if err == nil {
						state.Regions = html.EscapeString(string(regions))
					}
				}
			}()
		}
		experiments := h.Experiments.Load()
		if experiments != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				experiments, err := json.Marshal(experiments)
				if err == nil {
					state.Experiments = html.EscapeString(string(experiments))
				}
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			tasksTabVisible, err := json.Marshal(!h.opts.HideAITasks)
			if err == nil {
				state.TasksTabVisible = html.EscapeString(string(tasksTabVisible))
			}
		}()
		wg.Wait()
	}

	return execTmpl(tmpl, state)
}

// noopResponseWriter is a response writer that does nothing.
type noopResponseWriter struct{}

func (noopResponseWriter) Header() http.Header         { return http.Header{} }
func (noopResponseWriter) Write(p []byte) (int, error) { return len(p), nil }
func (noopResponseWriter) WriteHeader(int)             {}

// secureHeaders is only needed for statically served files. We do not need this for api endpoints.
// It adds various headers to enforce browser security features.
func secureHeaders() *secure.Secure {
	// Permissions-Policy can be used to disabled various browser features that we do not use.
	// This can prevent an embedded iframe from accessing these features.
	// If we support arbitrary iframes such as generic applications, we might need to add permissions
	// based on the app here.
	permissions := strings.Join([]string{
		// =() means it is disabled
		"accelerometer=()",
		"autoplay=()",
		"battery=()",
		"camera=()",
		"document-domain=()",
		"geolocation=()",
		"gyroscope=()",
		"magnetometer=()",
		"microphone=()",
		"midi=()",
		"payment=()",
		"usb=()",
		"vr=()",
		"screen-wake-lock=()",
		"xr-spatial-tracking=()",
	}, ", ")

	return secure.New(secure.Options{
		PermissionsPolicy: permissions,

		// Prevent the browser from sending Referrer header with requests
		ReferrerPolicy: "no-referrer",
	})
}

// findAndParseHTMLFiles recursively walks the file system passed finding all *.html files.
// The template returned has all html files parsed.
func findAndParseHTMLFiles(files fs.FS) (*template.Template, error) {
	// root is the collection of html templates. All templates are named by their pathing.
	// So './404.html' is named '404.html'. './subdir/index.html' is 'subdir/index.html'
	root := template.New("")

	rootPath := "."
	err := fs.WalkDir(files, rootPath, func(filePath string, directory fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if directory.IsDir() {
			return nil
		}

		if filepath.Ext(directory.Name()) != ".html" {
			return nil
		}

		file, err := files.Open(filePath)
		if err != nil {
			return err
		}

		data, err := io.ReadAll(file)
		if err != nil {
			return err
		}

		tPath := strings.TrimPrefix(filePath, rootPath+string(filepath.Separator))
		_, err = root.New(tPath).Parse(string(data))
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return root, nil
}

type installScriptState struct {
	Origin  string
	Version string
}

func parseInstallScript(files fs.FS, buildInfo codersdk.BuildInfoResponse, filename string) ([]byte, error) {
	scriptFile, err := fs.ReadFile(files, filename)
	if err != nil {
		return nil, err
	}

	script, err := template.New(filename).Parse(string(scriptFile))
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	state := installScriptState{Origin: buildInfo.DashboardURL, Version: buildInfo.Version}
	err = script.Execute(&buf, state)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Action represents a link.
type Action struct {
	// URL is set as the href property on the anchor.  If empty, refreshes the
	// page instead.
	URL string
	// Text is the displayed text of the button or link.
	Text string
}

// ErrorPageData contains the variables that are found in
// site/static/error.html.
type ErrorPageData struct {
	Status int
	// HideStatus will remove the status code from the page.
	HideStatus     bool
	Title          string
	Description    string
	Actions        []Action
	Warnings       []string
	AdditionalInfo string

	RenderDescriptionMarkdown bool
}

// RenderStaticErrorPage renders the static error page. This is used by app
// requests to avoid dependence on the dashboard but maintain the ability to
// render a friendly error page on subdomains.
func RenderStaticErrorPage(rw http.ResponseWriter, r *http.Request, data ErrorPageData) {
	type outerData struct {
		Error ErrorPageData

		ErrorDescriptionHTML htmltemplate.HTML
	}

	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.WriteHeader(data.Status)

	err := errorTemplate.Execute(rw, outerData{
		Error:                data,
		ErrorDescriptionHTML: htmltemplate.HTML(data.Description), //nolint:gosec // gosec thinks this is user-input, but it is from Coder deployment configuration.
	})
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to render error page: " + err.Error(),
			Detail:  fmt.Sprintf("Original error was: %d %s, %s", data.Status, data.Title, data.Description),
		})
		return
	}
}

func applicationNameOrDefault(cfg codersdk.AppearanceConfig) string {
	if cfg.ApplicationName != "" {
		return cfg.ApplicationName
	}
	return "Coder"
}

// OnlyFiles returns a new fs.FS that only contains files. If a directory is
// requested, os.ErrNotExist is returned. This prevents directory listings from
// being served.
func OnlyFiles(files fs.FS) fs.FS {
	return justFilesSystem{FS: files}
}

type justFilesSystem struct {
	FS fs.FS
}

func (jfs justFilesSystem) Open(name string) (fs.File, error) {
	f, err := jfs.FS.Open(name)
	if err != nil {
		return nil, err
	}

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}

	// Returning a 404 here does prevent the http.FileServer from serving
	// index.* files automatically. Coder handles this above as all index pages
	// are considered template files. So we never relied on this behavior.
	if stat.IsDir() {
		return nil, os.ErrNotExist
	}

	return f, nil
}

// RenderOAuthAllowData contains the variables that are found in
// site/static/oauth2allow.html.
type RenderOAuthAllowData struct {
	AppIcon     string
	AppName     string
	CancelURI   string
	RedirectURI string
	Username    string
}

// RenderOAuthAllowPage renders the static page for a user to "Allow" an create
// a new oauth2 link with an external site. This is when Coder is acting as the
// identity provider.
//
// This has to be done statically because Golang has to handle the full request.
// It cannot defer to the FE typescript easily.
func RenderOAuthAllowPage(rw http.ResponseWriter, r *http.Request, data RenderOAuthAllowData) {
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")

	err := oauthTemplate.Execute(rw, data)
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.Response{
			Message: "Failed to render oauth page: " + err.Error(),
		})
		return
	}
}
