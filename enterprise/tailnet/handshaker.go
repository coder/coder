package tailnet

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

type readyForHandshake struct {
	src uuid.UUID
	dst uuid.UUID
}

type handshaker struct {
	ctx           context.Context
	logger        slog.Logger
	coordinatorID uuid.UUID
	pubsub        pubsub.Pubsub
	updates       <-chan readyForHandshake

	workerWG sync.WaitGroup
}

func newHandshaker(ctx context.Context,
	logger slog.Logger,
	id uuid.UUID,
	ps pubsub.Pubsub,
	updates <-chan readyForHandshake,
	startWorkers <-chan struct{},
) *handshaker {
	s := &handshaker{
		ctx:           ctx,
		logger:        logger,
		coordinatorID: id,
		pubsub:        ps,
		updates:       updates,
	}
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

func (t *handshaker) worker() {
	defer t.workerWG.Done()

	for {
		select {
		case <-t.ctx.Done():
			t.logger.Debug(t.ctx, "handshaker worker exiting", slog.Error(t.ctx.Err()))
			return

		case rfh := <-t.updates:
			err := t.pubsub.Publish(eventReadyForHandshake, []byte(fmt.Sprintf(
				"%s,%s", rfh.dst.String(), rfh.src.String(),
			)))
			if err != nil {
				t.logger.Error(t.ctx, "publish ready for handshake", slog.Error(err))
			}
		}
	}
}
