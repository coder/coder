package site

import (
	"encoding/json"
	"fmt"
	"net/http"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
)

// webManifestApp describes an installable app served as a web app manifest.
type webManifestApp struct {
	// ID is both the manifest id and start_url. Browsers key installs by id,
	// so changing it orphans existing installs.
	ID string
	// Label is appended to the application name, and replaces it as the
	// short name. Empty for the dashboard.
	Label string
}

// webManifestApps maps request paths to the app they describe.
var webManifestApps = map[string]webManifestApp{
	"manifest.webmanifest":        {ID: "/"},
	"agents/manifest.webmanifest": {ID: "/agents", Label: "Agents"},
}

// webManifest is a web app manifest, see https://www.w3.org/TR/appmanifest/.
type webManifest struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	ShortName       string            `json:"short_name"`
	StartURL        string            `json:"start_url"`
	Scope           string            `json:"scope"`
	Display         string            `json:"display"`
	BackgroundColor string            `json:"background_color"`
	ThemeColor      string            `json:"theme_color"`
	Icons           []webManifestIcon `json:"icons"`
}

type webManifestIcon struct {
	Src     string `json:"src"`
	Sizes   string `json:"sizes"`
	Type    string `json:"type"`
	Purpose string `json:"purpose"`
}

// newWebManifest builds the manifest for app, named after the deployment's
// application name and access URL host, e.g. "Coder Agents (dev.coder.com)".
func newWebManifest(app webManifestApp, appName, host string) webManifest {
	name, shortName := appName, appName
	if app.Label != "" {
		name = appName + " " + app.Label
		shortName = app.Label
	}
	if host != "" {
		name = fmt.Sprintf("%s (%s)", name, host)
	}

	return webManifest{
		ID:              app.ID,
		Name:            name,
		ShortName:       shortName,
		StartURL:        app.ID,
		Scope:           "/",
		Display:         "standalone",
		BackgroundColor: "#17172E",
		ThemeColor:      "#17172E",
		Icons: []webManifestIcon{
			{Src: "/pwa-icon-192.png", Sizes: "192x192", Type: "image/png", Purpose: "any"},
			{Src: "/pwa-icon-512.png", Sizes: "512x512", Type: "image/png", Purpose: "any maskable"},
		},
	}
}

func (h *Handler) serveWebManifest(rw http.ResponseWriter, r *http.Request, app webManifestApp) {
	// nolint:gocritic // Browsers fetch manifests without credentials.
	ctx := dbauthz.AsSystemRestricted(r.Context())
	cfg, err := (*h.opts.AppearanceFetcher.Load()).Fetch(ctx)
	if err != nil {
		h.opts.Logger.Warn(ctx, "fetch appearance for web manifest", slog.Error(err))
		cfg = codersdk.AppearanceConfig{}
	}

	var host string
	if h.opts.AccessURL != nil {
		host = h.opts.AccessURL.Host
	}

	body, err := json.Marshal(newWebManifest(app, applicationNameOrDefault(cfg), host))
	if err != nil {
		http.Error(rw, "marshal web manifest", http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "application/manifest+json")
	// The name follows the appearance settings, so clients must revalidate.
	rw.Header().Set("Cache-Control", "no-cache")
	_, _ = rw.Write(body)
}
