package codersdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/netip"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/tailnet"
	"github.com/coder/retry"
)

type GoogleInstanceIdentityToken struct {
	JSONWebToken string `json:"json_web_token" validate:"required"`
}

type AWSInstanceIdentityToken struct {
	Signature string `json:"signature" validate:"required"`
	Document  string `json:"document" validate:"required"`
}

type AzureInstanceIdentityToken struct {
	Signature string `json:"signature" validate:"required"`
	Encoding  string `json:"encoding" validate:"required"`
}

// WorkspaceAgentAuthenticateResponse is returned when an instance ID
// has been exchanged for a session token.
type WorkspaceAgentAuthenticateResponse struct {
	SessionToken string `json:"session_token"`
}

// WorkspaceAgentConnectionInfo returns required information for establishing
// a connection with a workspace.
type WorkspaceAgentConnectionInfo struct {
	DERPMap *tailcfg.DERPMap `json:"derp_map"`
}

type PostWorkspaceAgentVersionRequest struct {
	Version string `json:"version"`
}

// AuthWorkspaceGoogleInstanceIdentity uses the Google Compute Engine Metadata API to
// fetch a signed JWT, and exchange it for a session token for a workspace agent.
//
// The requesting instance must be registered as a resource in the latest history for a workspace.
func (c *Client) AuthWorkspaceGoogleInstanceIdentity(ctx context.Context, serviceAccount string, gcpClient *metadata.Client) (WorkspaceAgentAuthenticateResponse, error) {
	if serviceAccount == "" {
		// This is the default name specified by Google.
		serviceAccount = "default"
	}
	if gcpClient == nil {
		gcpClient = metadata.NewClient(c.HTTPClient)
	}
	// "format=full" is required, otherwise the responding payload will be missing "instance_id".
	jwt, err := gcpClient.Get(fmt.Sprintf("instance/service-accounts/%s/identity?audience=coder&format=full", serviceAccount))
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, xerrors.Errorf("get metadata identity: %w", err)
	}
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/google-instance-identity", GoogleInstanceIdentityToken{
		JSONWebToken: jwt,
	})
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceAgentAuthenticateResponse{}, readBodyAsError(res)
	}
	var resp WorkspaceAgentAuthenticateResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// AuthWorkspaceAWSInstanceIdentity uses the Amazon Metadata API to
// fetch a signed payload, and exchange it for a session token for a workspace agent.
//
// The requesting instance must be registered as a resource in the latest history for a workspace.
func (c *Client) AuthWorkspaceAWSInstanceIdentity(ctx context.Context) (WorkspaceAgentAuthenticateResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://169.254.169.254/latest/api/token", nil)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, nil
	}
	req.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "21600")
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	token, err := io.ReadAll(res.Body)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, xerrors.Errorf("read token: %w", err)
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/latest/dynamic/instance-identity/signature", nil)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, nil
	}
	req.Header.Set("X-aws-ec2-metadata-token", string(token))
	res, err = c.HTTPClient.Do(req)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	signature, err := io.ReadAll(res.Body)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, xerrors.Errorf("read token: %w", err)
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/latest/dynamic/instance-identity/document", nil)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, nil
	}
	req.Header.Set("X-aws-ec2-metadata-token", string(token))
	res, err = c.HTTPClient.Do(req)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	document, err := io.ReadAll(res.Body)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, xerrors.Errorf("read token: %w", err)
	}

	res, err = c.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/aws-instance-identity", AWSInstanceIdentityToken{
		Signature: string(signature),
		Document:  string(document),
	})
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceAgentAuthenticateResponse{}, readBodyAsError(res)
	}
	var resp WorkspaceAgentAuthenticateResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// AuthWorkspaceAzureInstanceIdentity uses the Azure Instance Metadata Service to
// fetch a signed payload, and exchange it for a session token for a workspace agent.
func (c *Client) AuthWorkspaceAzureInstanceIdentity(ctx context.Context) (WorkspaceAgentAuthenticateResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/metadata/attested/document?api-version=2020-09-01", nil)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, nil
	}
	req.Header.Set("Metadata", "true")
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()

	var token AzureInstanceIdentityToken
	err = json.NewDecoder(res.Body).Decode(&token)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}

	res, err = c.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/azure-instance-identity", token)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceAgentAuthenticateResponse{}, readBodyAsError(res)
	}
	var resp WorkspaceAgentAuthenticateResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// WorkspaceAgentMetadata fetches metadata for the currently authenticated workspace agent.
func (c *Client) WorkspaceAgentMetadata(ctx context.Context) (agent.Metadata, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/workspaceagents/me/metadata", nil)
	if err != nil {
		return agent.Metadata{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return agent.Metadata{}, readBodyAsError(res)
	}
	var agentMetadata agent.Metadata
	return agentMetadata, json.NewDecoder(res.Body).Decode(&agentMetadata)
}

func (c *Client) ListenWorkspaceAgentTailnet(ctx context.Context) (net.Conn, error) {
	coordinateURL, err := c.URL.Parse("/api/v2/workspaceagents/me/coordinate")
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(coordinateURL, []*http.Cookie{{
		Name:  SessionTokenKey,
		Value: c.SessionToken,
	}})
	httpClient := &http.Client{
		Jar: jar,
	}
	// nolint:bodyclose
	conn, res, err := websocket.Dial(ctx, coordinateURL.String(), &websocket.DialOptions{
		HTTPClient: httpClient,
	})
	if err != nil {
		if res == nil {
			return nil, err
		}
		return nil, readBodyAsError(res)
	}

	return websocket.NetConn(ctx, conn, websocket.MessageBinary), nil
}

func (c *Client) DialWorkspaceAgentTailnet(ctx context.Context, logger slog.Logger, agentID uuid.UUID) (*agent.Conn, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceagents/%s/connection", agentID), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var connInfo WorkspaceAgentConnectionInfo
	err = json.NewDecoder(res.Body).Decode(&connInfo)
	if err != nil {
		return nil, xerrors.Errorf("decode conn info: %w", err)
	}

	ip := tailnet.IP()
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses: []netip.Prefix{netip.PrefixFrom(ip, 128)},
		DERPMap:   connInfo.DERPMap,
		Logger:    logger,
	})
	if err != nil {
		return nil, xerrors.Errorf("create tailnet: %w", err)
	}

	coordinateURL, err := c.URL.Parse(fmt.Sprintf("/api/v2/workspaceagents/%s/coordinate", agentID))
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(coordinateURL, []*http.Cookie{{
		Name:  SessionTokenKey,
		Value: c.SessionToken,
	}})
	httpClient := &http.Client{
		Jar: jar,
	}
	ctx, cancelFunc := context.WithCancel(ctx)
	closed := make(chan struct{})
	first := make(chan error)
	go func() {
		defer close(closed)
		isFirst := true
		for retrier := retry.New(50*time.Millisecond, 10*time.Second); retrier.Wait(ctx); {
			logger.Debug(ctx, "connecting")
			// nolint:bodyclose
			ws, res, err := websocket.Dial(ctx, coordinateURL.String(), &websocket.DialOptions{
				HTTPClient: httpClient,
				// Need to disable compression to avoid a data-race.
				CompressionMode: websocket.CompressionDisabled,
			})
			if errors.Is(err, context.Canceled) {
				return
			}
			if isFirst {
				if res.StatusCode == http.StatusConflict {
					first <- readBodyAsError(res)
					return
				}
				isFirst = false
				close(first)
			}
			if err != nil {
				logger.Debug(ctx, "failed to dial", slog.Error(err))
				continue
			}
			if isFirst {
				isFirst = false
				close(first)
			}
			sendNode, errChan := tailnet.ServeCoordinator(websocket.NetConn(ctx, ws, websocket.MessageBinary), func(node []*tailnet.Node) error {
				return conn.UpdateNodes(node)
			})
			conn.SetNodeCallback(sendNode)
			logger.Debug(ctx, "serving coordinator")
			err = <-errChan
			if errors.Is(err, context.Canceled) {
				_ = ws.Close(websocket.StatusGoingAway, "")
				return
			}
			if err != nil {
				logger.Debug(ctx, "error serving coordinator", slog.Error(err))
				_ = ws.Close(websocket.StatusGoingAway, "")
				continue
			}
			_ = ws.Close(websocket.StatusGoingAway, "")
		}
	}()
	err = <-first
	if err != nil {
		cancelFunc()
		_ = conn.Close()
		return nil, err
	}
	return &agent.Conn{
		Conn: conn,
		CloseFunc: func() {
			cancelFunc()
			<-closed
		},
	}, err
}

// WorkspaceAgent returns an agent by ID.
func (c *Client) WorkspaceAgent(ctx context.Context, id uuid.UUID) (WorkspaceAgent, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceagents/%s", id), nil)
	if err != nil {
		return WorkspaceAgent{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceAgent{}, readBodyAsError(res)
	}
	var workspaceAgent WorkspaceAgent
	return workspaceAgent, json.NewDecoder(res.Body).Decode(&workspaceAgent)
}

func (c *Client) PostWorkspaceAgentVersion(ctx context.Context, version string) error {
	// Phone home and tell the mothership what version we're on.
	versionReq := PostWorkspaceAgentVersionRequest{Version: version}
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/me/version", versionReq)
	if err != nil {
		return readBodyAsError(res)
	}
	// Discord the response
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	return nil
}

// WorkspaceAgentReconnectingPTY spawns a PTY that reconnects using the token provided.
// It communicates using `agent.ReconnectingPTYRequest` marshaled as JSON.
// Responses are PTY output that can be rendered.
func (c *Client) WorkspaceAgentReconnectingPTY(ctx context.Context, agentID, reconnect uuid.UUID, height, width int, command string) (net.Conn, error) {
	serverURL, err := c.URL.Parse(fmt.Sprintf("/api/v2/workspaceagents/%s/pty?reconnect=%s&height=%d&width=%d&command=%s", agentID, reconnect, height, width, command))
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
	conn, res, err := websocket.Dial(ctx, serverURL.String(), &websocket.DialOptions{
		HTTPClient: httpClient,
	})
	if err != nil {
		if res == nil {
			return nil, err
		}
		return nil, readBodyAsError(res)
	}
	return websocket.NetConn(ctx, conn, websocket.MessageBinary), nil
}

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
						_ = conn.Close(websocket.StatusGoingAway, "")
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
						_ = conn.Close(websocket.StatusGoingAway, "")
						return err
					}
				}
			}()
			if err != nil && ctx.Err() == nil {
				log.Error(ctx, "report stats", slog.Error(err))
			}
		}
	}()

	return closeFunc(func() error {
		cancel()
		<-doneCh
		return nil
	}), nil
}
