package main

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/nats"
)

// topology owns the embedded pubsub nodes for one benchmark run.
type topology struct {
	nodes []*nats.Pubsub
}

// closeAll shuts down every node. Pubsub.Close is idempotent and
// always returns nil.
func (t *topology) closeAll() {
	for _, node := range t.nodes {
		_ = node.Close()
	}
}

// staticPeerFetcher is a mutable nats.PeerFetcher seeded after every
// node in the cluster has started.
type staticPeerFetcher struct {
	mu    sync.Mutex
	addrs []string
}

func (*staticPeerFetcher) SetSelfNATSPort(int32) {}

var _ nats.PeerFetcher = (*staticPeerFetcher)(nil)

func (f *staticPeerFetcher) FetchNATSPeers() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.addrs)
}

func (f *staticPeerFetcher) set(addrs []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addrs = slices.Clone(addrs)
}

// buildTopology starts cfg.Replicas embedded pubsub nodes. For
// multi-replica runs it wires a full mesh: every node learns the other
// nodes' route addresses through its peer fetcher and refreshes its
// routes. Route convergence is not assumed here; the readiness gate
// proves it before the measured phase.
func buildTopology(ctx context.Context, logger slog.Logger, cfg Config) (*topology, error) {
	// The route auth token is unique per run so concurrent benchmark
	// processes on one host can never mesh with each other.
	token := fmt.Sprintf("natsbench-%d", time.Now().UnixNano())

	// One fetcher shared by every node: each node filters its own
	// address out of the route list, so they can all be handed the
	// full set of addresses.
	fetcher := &staticPeerFetcher{}
	top := &topology{nodes: make([]*nats.Pubsub, 0, cfg.Replicas)}
	for i := range cfg.Replicas {
		opts := pubsubOptions(cfg)
		opts.ClusterAuthToken = token
		opts.PeerFetcher = fetcher
		node, err := nats.New(ctx, logger.Named(fmt.Sprintf("node%d", i)), opts)
		if err != nil {
			top.closeAll()
			return nil, xerrors.Errorf("create node %d: %w", i, err)
		}
		top.nodes = append(top.nodes, node)
	}

	if cfg.Replicas > 1 {
		addrs := make([]string, len(top.nodes))
		for i, node := range top.nodes {
			addr, err := routeAddress(node)
			if err != nil {
				top.closeAll()
				return nil, xerrors.Errorf("node %d route address: %w", i, err)
			}
			addrs[i] = addr
		}
		fetcher.set(addrs)
		for _, node := range top.nodes {
			node.RefreshPeers()
		}
	}
	return top, nil
}

// pubsubOptions maps the benchmark config onto nats.Options. The
// cluster route listener always uses a random port: a zero ClusterPort
// means the production default 6222, which both collides across nodes
// and triggers peer-address port rewriting.
func pubsubOptions(cfg Config) nats.Options {
	opts := nats.Options{
		InProcess:      cfg.InProcess,
		PublishConns:   cfg.PublishConns,
		SubscribeConns: cfg.SubscribeConns,
		ClusterHost:    "127.0.0.1",
		ClusterPort:    natsserver.RANDOM_PORT,
	}
	if cfg.LocalQueueMsgs > 0 {
		opts.PendingLimits.Msgs = cfg.LocalQueueMsgs
	}
	if cfg.LocalQueueBytes > 0 {
		opts.PendingLimits.Bytes = cfg.LocalQueueBytes
	}
	if cfg.MaxPending > 0 {
		opts.MaxPending = cfg.MaxPending
	}
	return opts
}

// routeAddress returns the node's cluster route address as a NATS URL.
func routeAddress(node *nats.Pubsub) (string, error) {
	addr := node.Server.ClusterAddr()
	if addr == nil {
		return "", xerrors.New("server has no cluster listener")
	}
	return "nats://" + addr.String(), nil
}
