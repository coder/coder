package agentapi_test

import (
	"context"
	"testing"
	"time"

	"golang.org/x/xerrors"
	"storj.io/drpc"
	"tailscale.com/tailcfg"

	"github.com/stretchr/testify/require"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/tailnet"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
)

type fakeDERPMapStream struct {
	drpc.Stream // to fake implement unused members

	ctx     context.Context
	closeFn func() error
	sendFn  func(*tailnetproto.DERPMap) error
}

var _ agentproto.DRPCAgent_StreamDERPMapsStream = &fakeDERPMapStream{}

func (s *fakeDERPMapStream) Context() context.Context {
	return s.ctx
}

func (s *fakeDERPMapStream) Close() error {
	return s.closeFn()
}

func (s *fakeDERPMapStream) Send(m *tailnetproto.DERPMap) error {
	return s.sendFn(m)
}

func TestStreamDERPMaps(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		derpMap := tailcfg.DERPMap{}
		api := &agentapi.TailnetAPI{
			Ctx: context.Background(),
			DerpMapFn: func() *tailcfg.DERPMap {
				derp := (&derpMap).Clone()
				return derp
			},
			DerpMapUpdateFrequency: time.Millisecond,
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		closed := make(chan struct{})
		maps := make(chan *tailnetproto.DERPMap, 10)
		stream := &fakeDERPMapStream{
			ctx: ctx,
			closeFn: func() error {
				select {
				case <-ctx.Done():
				default:
					t.Fatal("expected context to be canceled before close")
				}
				close(closed)
				return nil
			},
			sendFn: func(m *tailnetproto.DERPMap) error {
				if m == nil {
					t.Fatal("expected non-nil map")
				}
				maps <- m
				return nil
			},
		}

		errCh := make(chan error)
		go func() {
			// Request isn't used.
			errCh <- api.StreamDERPMaps(nil, stream)
		}()

		// Initial map.
		gotMap := <-maps
		require.Equal(t, tailnet.DERPMapToProto(&derpMap), gotMap)

		// Update the map, should get an update.
		derpMap.Regions = map[int]*tailcfg.DERPRegion{
			1: {},
		}
		gotMap = <-maps
		require.Equal(t, tailnet.DERPMapToProto(&derpMap), gotMap)

		// Update the map again, should get an update.
		derpMap.Regions = nil
		gotMap = <-maps
		require.Equal(t, tailnet.DERPMapToProto(&derpMap), gotMap)

		// Cancel the stream, should return the fn.
		cancel()
		<-closed
		require.NoError(t, <-errCh)
	})

	t.Run("SendFailure", func(t *testing.T) {
		t.Parallel()

		api := &agentapi.TailnetAPI{
			Ctx: context.Background(),
			DerpMapFn: func() *tailcfg.DERPMap {
				return &tailcfg.DERPMap{}
			},
			DerpMapUpdateFrequency: time.Millisecond,
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		stream := &fakeDERPMapStream{
			ctx: ctx,
			closeFn: func() error {
				return nil
			},
			sendFn: func(m *tailnetproto.DERPMap) error {
				return xerrors.New("test error")
			},
		}

		err := api.StreamDERPMaps(nil, stream)
		require.Error(t, err)
		require.ErrorContains(t, err, "send derp map")
		require.ErrorContains(t, err, "test error")
	})

	t.Run("GlobalContextCanceled", func(t *testing.T) {
		t.Parallel()

		globalCtx, globalCtxCancel := context.WithCancel(context.Background())
		api := &agentapi.TailnetAPI{
			Ctx: globalCtx,
			DerpMapFn: func() *tailcfg.DERPMap {
				return &tailcfg.DERPMap{}
			},
			DerpMapUpdateFrequency: time.Hour, // long time to make sure ctx cancels are quick
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		maps := make(chan *tailnetproto.DERPMap, 10)
		stream := &fakeDERPMapStream{
			ctx: ctx,
			closeFn: func() error {
				return nil
			},
			sendFn: func(m *tailnetproto.DERPMap) error {
				if m == nil {
					t.Fatal("expected non-nil map")
				}
				maps <- m
				return nil
			},
		}

		errCh := make(chan error)
		go func() {
			// Request isn't used.
			errCh <- api.StreamDERPMaps(nil, stream)
		}()

		// Initial map.
		<-maps

		// Cancel the global context, should return the fn.
		globalCtxCancel()
		require.NoError(t, <-errCh)
	})
}
