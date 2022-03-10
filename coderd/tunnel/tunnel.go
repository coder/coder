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
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v2"
)

// New creates a new tunnel pointing at the URL provided.
// Once created, it returns the external hostname that will resolve to it.
//
// The tunnel will exit when the context provided is canceled.
//
// Upstream connection occurs async through Cloudflare, so the error channel
// will only be executed if the tunnel has failed after numerous attempts.
func New(ctx context.Context, url string) (string, <-chan error, error) {
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
	resp, err := client.Post("https://api.trycloudflare.com/tunnel", "application/json", nil)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to request quick Tunnel")
	}
	defer resp.Body.Close()

	var data quickTunnelResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", nil, errors.Wrap(err, "failed to unmarshal quick Tunnel")
	}

	tunnelID, err := uuid.Parse(data.Result.ID)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to parse quick Tunnel ID")
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
	appCtx.Set("url", url)
	appCtx.Set("protocol", "quic")
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
