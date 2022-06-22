package agent

import (
	"context"

	"golang.org/x/xerrors"
	"inet.af/netaddr"

	"cdr.dev/slog"
	"github.com/coder/coder/peer/peerwg"
)

func (a *agent) startWireguard(ctx context.Context, addrs []netaddr.IPPrefix) error {
	if a.wg != nil {
		_ = a.wg.Close()
		a.wg = nil
	}

	if !a.enableWireguard {
		return nil
	}

	// We can't create a wireguard network without these.
	if len(addrs) == 0 || a.listenWireguardPeers == nil || a.postKeys == nil {
		return xerrors.New("wireguard is enabled, but no addresses were provided or necessary functions were not provided")
	}

	wg, err := peerwg.NewWireguardNetwork(ctx, a.logger.Named("wireguard"), addrs)
	if err != nil {
		return xerrors.Errorf("create wireguard network: %w", err)
	}

	err = a.postKeys(ctx, PublicKeys{
		Public: wg.Private.Public(),
		Disco:  wg.Disco,
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
				a.logger.Info(ctx, "added wireguard peer", slog.F("peer", peer.Public.ShortString()), slog.Error(err))
			}

			listenClose()
		}
	}()

	a.wg = wg
	return nil
}
