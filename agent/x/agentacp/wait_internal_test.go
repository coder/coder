package agentacp

import (
	"context"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func waitTestSession(ctx context.Context, s *SessionFSM) (workspacesdk.ACPSessionResponse, error) {
	ticker := time.NewTicker(testutil.IntervalFast)
	defer ticker.Stop()
	for {
		r, err := s.Read(ctx, nil)
		if err != nil {
			return r, err
		}
		switch r.Session.Status {
		case workspacesdk.ACPSessionStatusIdle:
			return r, nil
		case workspacesdk.ACPSessionStatusError:
			return r, xerrors.New(r.Session.Error)
		}
		select {
		case <-ctx.Done():
			return r, ctx.Err()
		case <-ticker.C:
		}
	}
}
