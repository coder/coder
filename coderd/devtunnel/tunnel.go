package devtunnel

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	frpclient "github.com/fatedier/frp/client"
	frpconfig "github.com/fatedier/frp/pkg/config"
	frpconsts "github.com/fatedier/frp/pkg/consts"
	frpcrypto "github.com/fatedier/golib/crypto"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"
)

// New creates a new tunnel pointing at the URL provided.
// Once created, it returns the external hostname that will resolve to it.
//
// The tunnel will exit when the context provided is canceled.
//
// Upstream connection occurs async through Cloudflare, so the error channel
// will only be executed if the tunnel has failed after numerous attempts.
func New(ctx context.Context, coderurl *url.URL) (string, <-chan error, error) {
	frpcrypto.DefaultSalt = "frp"

	cfg := frpconfig.GetDefaultClientConf()
	cfg.ServerAddr = "34.133.27.233"
	cfg.ServerPort = 7000
	cfg.LogWay = "file"
	cfg.LogFile = "/dev/null"
	cfg.LogLevel = "warn"

	var (
		id        = uuid.NewString()
		subdomain = strings.ReplaceAll(namesgenerator.GetRandomName(1), "_", "-")
		portStr   = coderurl.Port()
	)
	if portStr == "" {
		portStr = "80"
	}

	port, err := strconv.ParseInt(portStr, 10, 64)
	if err != nil {
		return "", nil, xerrors.Errorf("parse port %q: %w", port, err)
	}

	httpcfg := map[string]frpconfig.ProxyConf{
		id: &frpconfig.HTTPProxyConf{
			BaseProxyConf: frpconfig.BaseProxyConf{
				ProxyName:      id,
				ProxyType:      frpconsts.HTTPProxy,
				UseEncryption:  false,
				UseCompression: false,
				LocalSvrConf: frpconfig.LocalSvrConf{
					LocalIP:   coderurl.Hostname(),
					LocalPort: int(port),
				},
			},
			DomainConf: frpconfig.DomainConf{
				SubDomain: subdomain,
			},
			Locations: []string{""},
		},
	}

	if err := httpcfg[id].CheckForCli(); err != nil {
		return "", nil, xerrors.Errorf("check for cli: %w", err)
	}

	svc, err := frpclient.NewService(cfg, httpcfg, nil, "")
	if err != nil {
		return "", nil, xerrors.Errorf("create new proxy service: %w", err)
	}

	ch := make(chan error, 1)
	go func() {
		err := svc.Run()
		ch <- err
		close(ch)
	}()
	go func() {
		select {
		case <-ctx.Done():
			svc.Close()
		}
	}()

	return fmt.Sprintf("https://%s.tunnel.coder.app", subdomain), ch, nil
}
