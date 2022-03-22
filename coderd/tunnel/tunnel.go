package tunnel

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cloudflare/cloudflared/cmd/cloudflared/cliutil"
	"github.com/cloudflare/cloudflared/cmd/cloudflared/tunnel"
	"github.com/cloudflare/cloudflared/connection"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

// New creates a new tunnel pointing at the URL provided.
// Once created, it returns the external hostname that will resolve to it.
//
// The tunnel will exit when the context provided is canceled.
//
// Upstream connection occurs async through Cloudflare, so the error channel
// will only be executed if the tunnel has failed after numerous attempts.
func New(ctx context.Context, url string) (string, <-chan error, error) {
	_ = os.Setenv("QUIC_GO_DISABLE_RECEIVE_BUFFER_WARNING", "true")

	httpTimeout := time.Second * 30
	client := http.Client{
		Transport: &http.Transport{
			TLSHandshakeTimeout:   httpTimeout,
			ResponseHeaderTimeout: httpTimeout,
		},
		Timeout: httpTimeout,
	}

	// Taken from:
	// https://github.com/cloudflare/cloudflared/blob/22cd8ceb8cf279afc1c412ae7f98308ffcfdd298/cmd/cloudflared/tunnel/quick_tunnel.go#L38
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.trycloudflare.com/tunnel", nil)
	if err != nil {
		return "", nil, xerrors.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, xerrors.Errorf("request quick tunnel: %w", err)
	}
	defer resp.Body.Close()
	var data quickTunnelResponse
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return "", nil, xerrors.Errorf("decode: %w", err)
	}
	tunnelID, err := uuid.Parse(data.Result.ID)
	if err != nil {
		return "", nil, xerrors.Errorf("parse tunnel id: %w", err)
	}

	credentials := connection.Credentials{
		AccountTag:   data.Result.AccountTag,
		TunnelSecret: data.Result.Secret,
		TunnelID:     tunnelID,
	}

	namedTunnel := &connection.NamedTunnelProperties{
		Credentials:    credentials,
		QuickTunnelUrl: data.Result.Hostname,
	}

	set := flag.NewFlagSet("", 0)
	set.String("protocol", "", "")
	set.String("url", "", "")
	set.Int("retries", 5, "")
	appCtx := cli.NewContext(&cli.App{}, set, nil)
	appCtx.Context = ctx
	_ = appCtx.Set("url", url)
	_ = appCtx.Set("protocol", "quic")
	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)
	errCh := make(chan error, 1)
	go func() {
		err := tunnel.StartServer(appCtx, &cliutil.BuildInfo{}, namedTunnel, &logger, false)
		errCh <- err
	}()
	if !strings.HasPrefix(data.Result.Hostname, "https://") {
		data.Result.Hostname = "https://" + data.Result.Hostname
	}
	return data.Result.Hostname, errCh, nil
}

type quickTunnelResponse struct {
	Success bool
	Result  quickTunnel
	Errors  []quickTunnelError
}

type quickTunnelError struct {
	Code    int
	Message string
}

type quickTunnel struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Hostname   string `json:"hostname"`
	AccountTag string `json:"account_tag"`
	Secret     []byte `json:"secret"`
}
