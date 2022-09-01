package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"

	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"github.com/google/uuid"

	"cdr.dev/slog"
	"github.com/coder/coder/agent"
	"github.com/coder/retry"
)

// AgentReportStats begins a stat streaming connection with the Coder server.
// It is resilient to network failures and intermittent coderd issues.
func (c *Client) AgentReportStats(
	ctx context.Context,
	log slog.Logger,
	stats func() *agent.Stats,
) (io.Closer, error) {
	serverURL, err := c.URL.Parse("/api/v2/workspaceagents/me/report-stats")
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, xerrors.Errorf("create cookie jar: %w", err)
	}

	jar.SetCookies(serverURL, []*http.Cookie{{
		Name:  SessionTokenKey,
		Value: c.SessionToken,
	}})

	httpClient := &http.Client{
		Jar: jar,
	}

	doneCh := make(chan struct{})
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		defer close(doneCh)

		// If the agent connection succeeds for a while, then fails, then succeeds
		// for a while (etc.) the retry may hit the maximum. This is a normal
		// case for long-running agents that experience coderd upgrades, so
		// we use a short maximum retry limit.
		for r := retry.New(time.Second, time.Minute); r.Wait(ctx); {
			err = func() error {
				conn, res, err := websocket.Dial(ctx, serverURL.String(), &websocket.DialOptions{
					HTTPClient: httpClient,
					// Need to disable compression to avoid a data-race.
					CompressionMode: websocket.CompressionDisabled,
				})
				if err != nil {
					if res == nil {
						return err
					}
					return readBodyAsError(res)
				}

				for {
					var req AgentStatsReportRequest
					err := wsjson.Read(ctx, conn, &req)
					if err != nil {
						return err
					}

					s := stats()

					resp := AgentStatsReportResponse{
						NumConns: s.NumConns,
						RxBytes:  s.RxBytes,
						TxBytes:  s.TxBytes,
					}

					err = wsjson.Write(ctx, conn, resp)
					if err != nil {
						return err
					}
				}
			}()
			if err != nil && ctx.Err() == nil {
				log.Error(ctx, "report stats", slog.Error(err))
			}
		}
	}()

	return CloseFunc(func() error {
		cancel()
		<-doneCh
		return nil
	}), nil
}

type DAUEntry struct {
	Date time.Time `json:"date"`
	DAUs int       `json:"daus"`
}

type TemplateDAUsResponse struct {
	Entries []DAUEntry `json:"entries"`
}

func (c *Client) TemplateDAUs(ctx context.Context, templateID uuid.UUID) (*TemplateDAUsResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/templates/%s/daus", templateID), nil)
	if err != nil {
		return nil, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}

	var resp TemplateDAUsResponse
	return &resp, json.NewDecoder(res.Body).Decode(&resp)
}

// @typescript-ignore AgentStatsReportRequest

// AgentStatsReportRequest is a WebSocket request by coderd
// to the agent for stats.
type AgentStatsReportRequest struct {
}

// AgentStatsReportResponse is returned for each report
// request by the agent.
type AgentStatsReportResponse struct {
	NumConns int64 `json:"num_comms"`
	// RxBytes is the number of received bytes.
	RxBytes int64 `json:"rx_bytes"`
	// TxBytes is the number of received bytes.
	TxBytes int64 `json:"tx_bytes"`
}
