package agentfake_test

import (
	"context"
	"sync"

	"storj.io/drpc"

	agentproto "github.com/coder/coder/v2/agent/proto"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
)

// fakeDialer is a minimal stand-in for agentsdk.Client used by agentfake
// tests. It exposes only ConnectRPC28WithRole and hands back a
// fakeAgentRPC that records the RPCs agentfake actually calls
// (UpdateLifecycle, GetManifest, BatchUpdateMetadata). Every other RPC
// on DRPCAgentClient28 will nil-panic via the embedded interface. If
// agentfake starts calling a new RPC, the test will tell us.
type fakeDialer struct {
	manifest *agentproto.Manifest

	mu       sync.Mutex
	metadata map[string]*agentproto.Metadata // keyed by Metadata.Key
}

func (f *fakeDialer) ConnectRPC28WithRole(_ context.Context, _ string) (
	agentproto.DRPCAgentClient28, tailnetproto.DRPCTailnetClient28, error,
) {
	return &fakeAgentRPC{parent: f, conn: newFakeDRPCConn()}, nil, nil
}

// Metadata returns a snapshot of the most recent value seen per key
// across all BatchUpdateMetadata calls.
func (f *fakeDialer) Metadata() map[string]*agentproto.Metadata {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]*agentproto.Metadata, len(f.metadata))
	for k, v := range f.metadata {
		out[k] = v
	}
	return out
}

// fakeAgentRPC implements only the RPCs agentfake uses. The embedded
// DRPCAgentClient28 is nil, so calls to any other method nil-panic,
// which is the desired signal during test development.
type fakeAgentRPC struct {
	agentproto.DRPCAgentClient28
	parent *fakeDialer
	conn   *fakeDRPCConn
}

func (r *fakeAgentRPC) DRPCConn() drpc.Conn { return r.conn }

func (*fakeAgentRPC) UpdateLifecycle(_ context.Context, req *agentproto.UpdateLifecycleRequest) (*agentproto.Lifecycle, error) {
	return req.Lifecycle, nil
}

func (r *fakeAgentRPC) GetManifest(_ context.Context, _ *agentproto.GetManifestRequest) (*agentproto.Manifest, error) {
	return r.parent.manifest, nil
}

func (r *fakeAgentRPC) BatchUpdateMetadata(_ context.Context, req *agentproto.BatchUpdateMetadataRequest) (*agentproto.BatchUpdateMetadataResponse, error) {
	r.parent.mu.Lock()
	defer r.parent.mu.Unlock()
	if r.parent.metadata == nil {
		r.parent.metadata = make(map[string]*agentproto.Metadata)
	}
	for _, m := range req.Metadata {
		r.parent.metadata[m.Key] = m
	}
	return &agentproto.BatchUpdateMetadataResponse{}, nil
}

// fakeDRPCConn is the minimum drpc.Conn agentfake's connectAndServe
// needs: a Closed() channel it can select on, and a no-op Close().
// We never actually invoke any drpc RPCs through this conn; the
// fakeAgentRPC methods are called directly, so Invoke/NewStream
// panic if something tries to use them.
type fakeDRPCConn struct {
	closed chan struct{}
	once   sync.Once
}

func newFakeDRPCConn() *fakeDRPCConn { return &fakeDRPCConn{closed: make(chan struct{})} }

func (c *fakeDRPCConn) Close() error            { c.once.Do(func() { close(c.closed) }); return nil }
func (c *fakeDRPCConn) Closed() <-chan struct{} { return c.closed }
func (*fakeDRPCConn) Invoke(_ context.Context, _ string, _ drpc.Encoding, _, _ drpc.Message) error {
	panic("fakeDRPCConn.Invoke called; add a fakeAgentRPC method instead")
}

func (*fakeDRPCConn) NewStream(_ context.Context, _ string, _ drpc.Encoding) (drpc.Stream, error) {
	panic("fakeDRPCConn.NewStream called; add a fakeAgentRPC method instead")
}
