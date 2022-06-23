package agent

import (
	"context"

	"golang.org/x/xerrors"
	"inet.af/netaddr"

	"cdr.dev/slog"
	"github.com/coder/coder/peer/peerwg"
)

func (a *agent) startWireguard(ctx context.Context, addrs []netaddr.IPPrefix) error {
	if a.network != nil {
		_ = a.network.Close()
		a.network = nil
	}

	// We can't create a wireguard network without these.
	if len(addrs) == 0 || a.listenWireguardPeers == nil || a.postKeys == nil {
		return xerrors.New("wireguard is enabled, but no addresses were provided or necessary functions were not provided")
	}

	wg, err := peerwg.New(a.logger.Named("wireguard"), addrs)
	if err != nil {
		return xerrors.Errorf("create wireguard network: %w", err)
	}

	// A new keypair is generated on each agent start.
	// This keypair must be sent to Coder to allow for incoming connections.
	err = a.postKeys(ctx, WireguardPublicKeys{
		Public: wg.NodePrivateKey.Public(),
		Disco:  wg.DiscoPublicKey,
	})
	if err != nil {
		a.logger.Warn(ctx, "post keys", slog.Error(err))
	}

	go func() {
		for {
			ch, listenClose, err := a.listenWireguardPeers(ctx, a.logger)
			if err != nil {
				a.logger.Warn(ctx, "listen wireguard peers", slog.Error(err))
				return
			}

			for {
				peer, ok := <-ch
				if !ok {
					break
				}

				err := wg.AddPeer(peer)
				a.logger.Info(ctx, "added wireguard peer", slog.F("peer", peer.NodePublicKey.ShortString()), slog.Error(err))
			}

			listenClose()
		}
	}()

	a.network = wg
	return nil
}
