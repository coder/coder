package devtunnel

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	frpclient "github.com/fatedier/frp/client"
	frpconfig "github.com/fatedier/frp/pkg/config"
	frpconsts "github.com/fatedier/frp/pkg/consts"
	frplog "github.com/fatedier/frp/pkg/util/log"
	frpcrypto "github.com/fatedier/golib/crypto"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"
)

// New creates a new tunnel pointing at the URL provided. Once created, it
// returns the external hostname that will resolve to it.
//
// The tunnel will exit when the context provided is canceled.
//
// Upstream connection occurs synchronously through a selfhosted
// https://github.com/fatedier/frp instance. The error channel sends an error
// when the frp client stops.
func New(ctx context.Context, coderurl *url.URL) (string, <-chan error, error) {
	frpcrypto.DefaultSalt = "frp"

	cfg := frpconfig.GetDefaultClientConf()
	cfg.ServerAddr = "frp-tunnel.coder.app"
	cfg.ServerPort = 7000

	// Ignore all logs from frp.
	frplog.InitLog("file", os.DevNull, "error", -1, false)

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
		<-ctx.Done()
		svc.Close()
	}()

	return fmt.Sprintf("https://%s.try.coder.app", subdomain), ch, nil
}
