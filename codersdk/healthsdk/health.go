package healthsdk

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/xerrors"
	"tailscale.com/derp"
	"tailscale.com/net/netcheck"
	"tailscale.com/tailcfg"

	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/codersdk"
)

// @typescript-ignore Client
type Client struct {
	client *codersdk.Client
}

func NewClient(c *codersdk.Client) *Client {
	return &Client{client: c}
}

type Section string

// If you add another const below, make sure to add it to HealthSections!
const (
	SectionDERP               Section = "DERP"
	SectionAccessURL          Section = "AccessURL"
	SectionWebsocket          Section = "Websocket"
	SectionDatabase           Section = "Database"
	SectionWorkspaceProxy     Section = "WorkspaceProxy"
	SectionProvisionerDaemons Section = "ProvisionerDaemons"
)

var Sections = []Section{
	SectionDERP,
	SectionAccessURL,
	SectionWebsocket,
	SectionDatabase,
	SectionWorkspaceProxy,
	SectionProvisionerDaemons,
}

type Settings struct {
	DismissedHealthchecks []Section `json:"dismissed_healthchecks"`
}

type UpdateSettings struct {
	DismissedHealthchecks []Section `json:"dismissed_healthchecks"`
}

func (c *Client) DebugHealth(ctx context.Context) (HealthcheckReport, error) {
	res, err := c.client.Request(ctx, http.MethodGet, "/api/v2/debug/health", nil)
	if err != nil {
		return HealthcheckReport{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return HealthcheckReport{}, codersdk.ReadBodyAsError(res)
	}
	var rpt HealthcheckReport
	return rpt, json.NewDecoder(res.Body).Decode(&rpt)
}

func (c *Client) Settings(ctx context.Context) (Settings, error) {
	res, err := c.client.Request(ctx, http.MethodGet, "/api/v2/debug/health/settings", nil)
	if err != nil {
		return Settings{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Settings{}, codersdk.ReadBodyAsError(res)
	}
	var settings Settings
	return settings, json.NewDecoder(res.Body).Decode(&settings)
}

func (c *Client) PutSettings(ctx context.Context, settings Settings) error {
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

type HealthcheckReport struct {
	// Time is the time the report was generated at.
	Time time.Time `json:"time" format:"date-time"`
	// Healthy is true if the report returns no errors.
	// Deprecated: use `Severity` instead
	Healthy bool `json:"healthy"`
	// Severity indicates the status of Coder health.
	Severity health.Severity `json:"severity" enums:"ok,warning,error"`
	// FailingSections is a list of sections that have failed their healthcheck.
	FailingSections []Section `json:"failing_sections"`

	DERP               DERPHealthReport         `json:"derp"`
	AccessURL          AccessURLReport          `json:"access_url"`
	Websocket          WebsocketReport          `json:"websocket"`
	Database           DatabaseReport           `json:"database"`
	WorkspaceProxy     WorkspaceProxyReport     `json:"workspace_proxy"`
	ProvisionerDaemons ProvisionerDaemonsReport `json:"provisioner_daemons"`

	// The Coder version of the server that the report was generated on.
	CoderVersion string `json:"coder_version"`
}

type AccessURLReport struct {
	// Healthy is deprecated and left for backward compatibility purposes, use `Severity` instead.
	Healthy   bool             `json:"healthy"`
	Severity  health.Severity  `json:"severity" enums:"ok,warning,error"`
	Warnings  []health.Message `json:"warnings"`
	Dismissed bool             `json:"dismissed"`

	AccessURL       string  `json:"access_url"`
	Reachable       bool    `json:"reachable"`
	StatusCode      int     `json:"status_code"`
	HealthzResponse string  `json:"healthz_response"`
	Error           *string `json:"error"`
}

type DERPHealthReport struct {
	// Healthy is deprecated and left for backward compatibility purposes, use `Severity` instead.
	Healthy   bool             `json:"healthy"`
	Severity  health.Severity  `json:"severity" enums:"ok,warning,error"`
	Warnings  []health.Message `json:"warnings"`
	Dismissed bool             `json:"dismissed"`

	Regions map[int]*DERPRegionReport `json:"regions"`

	Netcheck     *netcheck.Report `json:"netcheck"`
	NetcheckErr  *string          `json:"netcheck_err"`
	NetcheckLogs []string         `json:"netcheck_logs"`

	Error *string `json:"error"`
}

type DERPRegionReport struct {
	// Healthy is deprecated and left for backward compatibility purposes, use `Severity` instead.
	Healthy  bool             `json:"healthy"`
	Severity health.Severity  `json:"severity" enums:"ok,warning,error"`
	Warnings []health.Message `json:"warnings"`

	Region      *tailcfg.DERPRegion `json:"region"`
	NodeReports []*DERPNodeReport   `json:"node_reports"`
	Error       *string             `json:"error"`
}

type DERPNodeReport struct {
	// Healthy is deprecated and left for backward compatibility purposes, use `Severity` instead.
	Healthy  bool             `json:"healthy"`
	Severity health.Severity  `json:"severity" enums:"ok,warning,error"`
	Warnings []health.Message `json:"warnings"`

	Node *tailcfg.DERPNode `json:"node"`

	ServerInfo          derp.ServerInfoMessage `json:"node_info"`
	CanExchangeMessages bool                   `json:"can_exchange_messages"`
	RoundTripPing       string                 `json:"round_trip_ping"`
	RoundTripPingMs     int                    `json:"round_trip_ping_ms"`
	UsesWebsocket       bool                   `json:"uses_websocket"`
	ClientLogs          [][]string             `json:"client_logs"`
	ClientErrs          [][]string             `json:"client_errs"`
	Error               *string                `json:"error"`

	STUN STUNReport `json:"stun"`
}

type STUNReport struct {
	Enabled bool
	CanSTUN bool
	Error   *string
}

type DatabaseReport struct {
	// Healthy is deprecated and left for backward compatibility purposes, use `Severity` instead.
	Healthy   bool             `json:"healthy"`
	Severity  health.Severity  `json:"severity" enums:"ok,warning,error"`
	Warnings  []health.Message `json:"warnings"`
	Dismissed bool             `json:"dismissed"`

	Reachable   bool    `json:"reachable"`
	Latency     string  `json:"latency"`
	LatencyMS   int64   `json:"latency_ms"`
	ThresholdMS int64   `json:"threshold_ms"`
	Error       *string `json:"error"`
}

type ProvisionerDaemonsReport struct {
	Severity  health.Severity  `json:"severity"`
	Warnings  []health.Message `json:"warnings"`
	Dismissed bool             `json:"dismissed"`
	Error     *string          `json:"error"`

	Items []ProvisionerDaemonsReportItem `json:"items"`
}

type ProvisionerDaemonsReportItem struct {
	codersdk.ProvisionerDaemon `json:"provisioner_daemon"`
	Warnings                   []health.Message `json:"warnings"`
}

type WebsocketReport struct {
	// Healthy is deprecated and left for backward compatibility purposes, use `Severity` instead.
	Healthy   bool            `json:"healthy"`
	Severity  health.Severity `json:"severity" enums:"ok,warning,error"`
	Warnings  []string        `json:"warnings"`
	Dismissed bool            `json:"dismissed"`

	Body  string  `json:"body"`
	Code  int     `json:"code"`
	Error *string `json:"error"`
}

type WorkspaceProxyReport struct {
	Healthy   bool             `json:"healthy"`
	Severity  health.Severity  `json:"severity"`
	Warnings  []health.Message `json:"warnings"`
	Dismissed bool             `json:"dismissed"`
	Error     *string          `json:"error"`

	WorkspaceProxies codersdk.RegionsResponse[codersdk.WorkspaceProxy] `json:"workspace_proxies"`
}
