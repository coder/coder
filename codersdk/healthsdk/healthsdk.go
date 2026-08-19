package healthsdk

import (
	"context"
	"net/http"
	"strings"
	"time"

	"golang.org/x/xerrors"
	"tailscale.com/derp"
	"tailscale.com/net/netcheck"
	"tailscale.com/tailcfg"

	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/codersdk"
)

// @typescript-ignore HealthClient
type HealthClient struct {
	client *codersdk.Client
}

func New(c *codersdk.Client) *HealthClient {
	return &HealthClient{client: c}
}

type HealthSection string

// If you add another const below, make sure to add it to HealthSections!
const (
	HealthSectionDERP               HealthSection = "DERP"
	HealthSectionAccessURL          HealthSection = "AccessURL"
	HealthSectionWebsocket          HealthSection = "Websocket"
	HealthSectionDatabase           HealthSection = "Database"
	HealthSectionWorkspaceProxy     HealthSection = "WorkspaceProxy"
	HealthSectionProvisionerDaemons HealthSection = "ProvisionerDaemons"
	HealthSectionUsagePublishing    HealthSection = "UsagePublishing"
)

var HealthSections = []HealthSection{
	HealthSectionDERP,
	HealthSectionAccessURL,
	HealthSectionWebsocket,
	HealthSectionDatabase,
	HealthSectionWorkspaceProxy,
	HealthSectionProvisionerDaemons,
	HealthSectionUsagePublishing,
}

type HealthSettings struct {
	DismissedHealthchecks []HealthSection `json:"dismissed_healthchecks"`
}

type UpdateHealthSettings struct {
	DismissedHealthchecks []HealthSection `json:"dismissed_healthchecks"`
}

func (c *HealthClient) DebugHealth(ctx context.Context) (HealthcheckReport, error) {
	res, err := c.client.Request(ctx, http.MethodGet, "/api/v2/debug/health", nil)
	if err != nil {
		return HealthcheckReport{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return HealthcheckReport{}, codersdk.ReadBodyAsError(res)
	}
	var rpt HealthcheckReport
	return rpt, codersdk.ReadBodyAsJSON(res, &rpt)
}

func (c *HealthClient) HealthSettings(ctx context.Context) (HealthSettings, error) {
	res, err := c.client.Request(ctx, http.MethodGet, "/api/v2/debug/health/settings", nil)
	if err != nil {
		return HealthSettings{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return HealthSettings{}, codersdk.ReadBodyAsError(res)
	}
	var settings HealthSettings
	return settings, codersdk.ReadBodyAsJSON(res, &settings)
}

func (c *HealthClient) PutHealthSettings(ctx context.Context, settings HealthSettings) error {
	res, err := c.client.Request(ctx, http.MethodPut, "/api/v2/debug/health/settings", settings)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNoContent {
		return xerrors.New("health settings not modified")
	}
	if res.StatusCode != http.StatusOK {
		return codersdk.ReadBodyAsError(res)
	}
	return nil
}

// HealthcheckReport contains information about the health status of a Coder deployment.
type HealthcheckReport struct {
	// Time is the time the report was generated at.
	Time time.Time `json:"time" format:"date-time"`
	// Healthy is true if the report returns no errors.
	// Deprecated: use `Severity` instead
	Healthy bool `json:"healthy"`
	// Severity indicates the status of Coder health.
	Severity health.Severity `json:"severity" enums:"ok,warning,error"`

	DERP               DERPHealthReport         `json:"derp"`
	AccessURL          AccessURLReport          `json:"access_url"`
	Websocket          WebsocketReport          `json:"websocket"`
	Database           DatabaseReport           `json:"database"`
	WorkspaceProxy     WorkspaceProxyReport     `json:"workspace_proxy"`
	ProvisionerDaemons ProvisionerDaemonsReport `json:"provisioner_daemons"`
	UsagePublishing    UsagePublishingReport    `json:"usage_publishing"`

	// The Coder version of the server that the report was generated on.
	CoderVersion string `json:"coder_version"`
}

// Summarize returns a summary of all errors and warnings of components of HealthcheckReport.
func (r *HealthcheckReport) Summarize(docsURL string) []string {
	var msgs []string
	msgs = append(msgs, r.AccessURL.Summarize("Access URL:", docsURL)...)
	msgs = append(msgs, r.Database.Summarize("Database:", docsURL)...)
	msgs = append(msgs, r.DERP.Summarize("DERP:", docsURL)...)
	msgs = append(msgs, r.ProvisionerDaemons.Summarize("Provisioner Daemons:", docsURL)...)
	msgs = append(msgs, r.Websocket.Summarize("Websocket:", docsURL)...)
	msgs = append(msgs, r.WorkspaceProxy.Summarize("Workspace Proxies:", docsURL)...)
	msgs = append(msgs, r.UsagePublishing.Summarize("Usage Publishing:", docsURL)...)
	return msgs
}

// BaseReport holds fields common to various health reports.
type BaseReport struct {
	Error     *string          `json:"error,omitempty"`
	Severity  health.Severity  `json:"severity" enums:"ok,warning,error"`
	Warnings  []health.Message `json:"warnings"`
	Dismissed bool             `json:"dismissed"`
}

// Summarize returns a list of strings containing the errors and warnings of BaseReport, if present.
// All strings are prefixed with prefix.
func (b *BaseReport) Summarize(prefix, docsURL string) []string {
	if b == nil {
		return []string{}
	}
	var msgs []string
	if b.Error != nil {
		var sb strings.Builder
		if prefix != "" {
			_, _ = sb.WriteString(prefix)
			_, _ = sb.WriteString(" ")
		}
		_, _ = sb.WriteString("Error: ")
		_, _ = sb.WriteString(*b.Error)
		msgs = append(msgs, sb.String())
	}
	for _, warn := range b.Warnings {
		var sb strings.Builder
		if prefix != "" {
			_, _ = sb.WriteString(prefix)
			_, _ = sb.WriteString(" ")
		}
		_, _ = sb.WriteString("Warn: ")
		_, _ = sb.WriteString(warn.String())
		msgs = append(msgs, sb.String())
		msgs = append(msgs, "See: "+warn.URL(docsURL))
	}
	return msgs
}

// AccessURLReport shows the results of performing a HTTP_GET to the /healthz endpoint through the configured access URL.
type AccessURLReport struct {
	BaseReport
	// Healthy is deprecated and left for backward compatibility purposes, use `Severity` instead.
	Healthy         bool   `json:"healthy"`
	AccessURL       string `json:"access_url"`
	Reachable       bool   `json:"reachable"`
	StatusCode      int    `json:"status_code"`
	HealthzResponse string `json:"healthz_response"`
}

// DERPHealthReport includes health details of each configured DERP/STUN region.
type DERPHealthReport struct {
	BaseReport
	// Healthy is deprecated and left for backward compatibility purposes, use `Severity` instead.
	Healthy      bool                      `json:"healthy"`
	Regions      map[int]*DERPRegionReport `json:"regions"`
	Netcheck     *netcheck.Report          `json:"netcheck,omitempty"`
	NetcheckErr  *string                   `json:"netcheck_err,omitempty"`
	NetcheckLogs []string                  `json:"netcheck_logs"`
}

// DERPHealthReport includes health details of each node in a single region.
type DERPRegionReport struct {
	// Healthy is deprecated and left for backward compatibility purposes, use `Severity` instead.
	Healthy     bool                `json:"healthy"`
	Severity    health.Severity     `json:"severity" enums:"ok,warning,error"`
	Warnings    []health.Message    `json:"warnings"`
	Error       *string             `json:"error,omitempty"`
	Region      *tailcfg.DERPRegion `json:"region"`
	NodeReports []*DERPNodeReport   `json:"node_reports"`
}

// DERPHealthReport includes health details of a single node in a single region.
type DERPNodeReport struct {
	// Healthy is deprecated and left for backward compatibility purposes, use `Severity` instead.
	Healthy  bool             `json:"healthy"`
	Severity health.Severity  `json:"severity" enums:"ok,warning,error"`
	Warnings []health.Message `json:"warnings"`
	Error    *string          `json:"error,omitempty"`

	Node *tailcfg.DERPNode `json:"node"`

	ServerInfo          derp.ServerInfoMessage `json:"node_info"`
	CanExchangeMessages bool                   `json:"can_exchange_messages"`
	RoundTripPing       string                 `json:"round_trip_ping"`
	RoundTripPingMs     int                    `json:"round_trip_ping_ms"`
	UsesWebsocket       bool                   `json:"uses_websocket"`
	ClientLogs          [][]string             `json:"client_logs"`
	ClientErrs          [][]string             `json:"client_errs"`

	STUN STUNReport `json:"stun"`
}

// STUNReport contains information about a given node's STUN capabilities.
type STUNReport struct {
	Enabled bool
	CanSTUN bool
	Error   *string
}

// DatabaseReport shows the results of pinging the configured database.Conn.
type DatabaseReport struct {
	BaseReport
	// Healthy is deprecated and left for backward compatibility purposes, use `Severity` instead.
	Healthy     bool   `json:"healthy"`
	Reachable   bool   `json:"reachable"`
	Latency     string `json:"latency"`
	LatencyMS   int64  `json:"latency_ms"`
	ThresholdMS int64  `json:"threshold_ms"`
}

// ProvisionerDaemonsReport includes health details of each connected provisioner daemon.
type ProvisionerDaemonsReport struct {
	BaseReport
	Items []ProvisionerDaemonsReportItem `json:"items"`
}

type ProvisionerDaemonsReportItem struct {
	codersdk.ProvisionerDaemon `json:"provisioner_daemon"`
	Warnings                   []health.Message `json:"warnings"`
}

// WebsocketReport shows if the configured access URL allows establishing WebSocket connections.
type WebsocketReport struct {
	// Healthy is deprecated and left for backward compatibility purposes, use `Severity` instead.
	Healthy bool `json:"healthy"`
	BaseReport
	Body string `json:"body"`
	Code int    `json:"code"`
}

// UsagePublishingReport shows the status of usage event publishing to
// Coder's servers. Deployments without a license that enables usage
// publishing (e.g. air-gapped deployments) always report as healthy with
// PublishingEnabled set to false.
type UsagePublishingReport struct {
	// Healthy is deprecated and left for backward compatibility purposes, use `Severity` instead.
	Healthy bool `json:"healthy"`
	BaseReport
	// PublishingEnabled is true if a currently-valid license enables usage
	// event publishing.
	PublishingEnabled bool `json:"publishing_enabled"`
	// LastPublishedAt is the time of the latest successful publish of a usage
	// event. It is null if there is no successful publish among the most
	// recent publish outcomes, e.g. because nothing has ever been published
	// or a sustained rejection streak displaced the last success.
	LastPublishedAt *time.Time `json:"last_published_at,omitempty" format:"date-time"`
	// StatusUnavailable is true when the latest entitlements refresh could
	// not determine the publishing status; this section's health is unknown
	// rather than OK.
	StatusUnavailable bool `json:"status_unavailable,omitempty"`
	// FailingSince is set when usage event publishing is considered failing.
	FailingSince *time.Time `json:"failing_since,omitempty" format:"date-time"`
}

// WorkspaceProxyReport includes health details of each connected workspace proxy.
type WorkspaceProxyReport struct {
	// Healthy is deprecated and left for backward compatibility purposes, use `Severity` instead.
	Healthy bool `json:"healthy"`
	BaseReport
	WorkspaceProxies codersdk.RegionsResponse[codersdk.WorkspaceProxy] `json:"workspace_proxies"`
}

// @typescript-ignore ClientNetcheckReport
type ClientNetcheckReport struct {
	DERP       DERPHealthReport `json:"derp"`
	Interfaces InterfacesReport `json:"interfaces"`
}

// @typescript-ignore AgentNetcheckReport
type AgentNetcheckReport struct {
	BaseReport
	NetInfo    *tailcfg.NetInfo `json:"net_info"`
	Interfaces InterfacesReport `json:"interfaces"`
}
