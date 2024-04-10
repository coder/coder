package tailnet

import (
	"context"
	"slices"
	"sync"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
)

type readyForHandshake struct {
	hKey
}

type hKey struct {
	src uuid.UUID
	dst uuid.UUID
}

type handshaker struct {
	ctx           context.Context
	logger        slog.Logger
	coordinatorID uuid.UUID
	store         database.Store
	updates       <-chan readyForHandshake

	workQ *workQ[hKey]

	workerWG sync.WaitGroup
}

func newHandshaker(ctx context.Context,
	logger slog.Logger,
	id uuid.UUID,
	store database.Store,
	updates <-chan readyForHandshake,
	startWorkers <-chan struct{},
) *handshaker {
	s := &handshaker{
		ctx:           ctx,
		logger:        logger,
		coordinatorID: id,
		store:         store,
		updates:       updates,
		workQ:         newWorkQ[hKey](ctx),
	}
	go s.handle()
	// add to the waitgroup immediately to avoid any races waiting for it before
	// the workers start.
	s.workerWG.Add(numHandshakerWorkers)
	go func() {
		<-startWorkers
		for i := 0; i < numHandshakerWorkers; i++ {
			go s.worker()
		}
	}()
	return s
}

func (t *handshaker) handle() {
	for {
		select {
		case <-t.ctx.Done():
			t.logger.Debug(t.ctx, "handshaker exiting", slog.Error(t.ctx.Err()))
			return
		case rfh := <-t.updates:
			t.workQ.enqueue(rfh.hKey)
		}
	}
}

func (t *handshaker) worker() {
	defer t.workerWG.Done()
	eb := backoff.NewExponentialBackOff()
	eb.MaxElapsedTime = 0 // retry indefinitely
	eb.MaxInterval = dbMaxBackoff
	bkoff := backoff.WithContext(eb, t.ctx)
	for {
		hk, err := t.workQ.acquire()
		if err != nil {
			// context expired
			return
		}
		err = backoff.Retry(func() error {
			return t.writeOne(hk)
		}, bkoff)
		if err != nil {
			bkoff.Reset()
		}
		t.workQ.done(hk)
	}
}

func (t *handshaker) writeOne(hk hKey) error {
	logger := t.logger.With(
		slog.F("src_id", hk.src),
		slog.F("dst_id", hk.dst),
	)

	peers, err := t.store.GetTailnetTunnelPeerIDs(t.ctx, hk.src)
	if err != nil {
		if !database.IsQueryCanceledError(err) {
			logger.Error(t.ctx, "get tunnel peers ids", slog.Error(err))
		}
		return err
	}

	if !slices.ContainsFunc(peers, func(peer database.GetTailnetTunnelPeerIDsRow) bool {
		return peer.PeerID == hk.dst
	}) {
		// In the in-memory coordinator we return an error to the client, but
		// this isn't really possible here.
		logger.Warn(t.ctx, "cannot process ready for handshake, src isn't peered with dst")
		return nil
	}

	err = t.store.PublishReadyForHandshake(t.ctx, database.PublishReadyForHandshakeParams{
		To:   hk.dst.String(),
		From: hk.src.String(),
	})
	if err != nil {
		if !database.IsQueryCanceledError(err) {
			logger.Error(t.ctx, "publish ready for handshake", slog.Error(err))
		}
		return err
	}

	return nil
}
