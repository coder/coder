package main

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	codernats "github.com/coder/coder/v2/coderd/x/nats"
)

// freeBenchPort grabs a free loopback TCP port and immediately closes
// the listener. There is an inherent race between close and reuse, but
// for the bench harness this is acceptable.
func freeBenchPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, xerrors.Errorf("listen 127.0.0.1:0: %w", err)
	}
	addr, ok := l.Addr().(*net.TCPAddr)
	_ = l.Close()
	if !ok {
		return 0, xerrors.Errorf("listener addr is not *net.TCPAddr: %T", l.Addr())
	}
	return addr.Port, nil
}

// startNativeCluster brings up n embedded nats-servers in a full-mesh
// cluster using bare natsserver options. Mirrors the pattern used by
// the bench tests in coderd/x/nats/bench_test.go. The caller is
// responsible for shutting each returned server down.
// maxPending is the per-client outbound pending byte budget for each
// replica. Pass 0 to keep the existing default (1 GiB); pass a positive
// value to override (e.g., 128 MiB for the symmetric cluster modes that
// bound worst-case in-flight bytes in fan-out scenarios).
func startNativeCluster(n int, maxPending int64) ([]*natsserver.Server, error) {
	if n < 1 {
		return nil, xerrors.Errorf("native cluster requires n >= 1, got %d", n)
	}
	if maxPending <= 0 {
		maxPending = 1 << 30
	}
	ports := make([]int, n)
	routes := make([]string, n)
	for i := 0; i < n; i++ {
		p, err := freeBenchPort()
		if err != nil {
			return nil, xerrors.Errorf("alloc route port %d: %w", i, err)
		}
		ports[i] = p
		routes[i] = "nats://127.0.0.1:" + strconv.Itoa(p)
	}
	// Full mesh: every server lists every peer's route URL (excluding self).
	parseURLs := func(self int) ([]*url.URL, error) {
		urls := make([]*url.URL, 0, n-1)
		for i, r := range routes {
			if i == self {
				continue
			}
			u, err := url.Parse(r)
			if err != nil {
				return nil, xerrors.Errorf("parse route %q: %w", r, err)
			}
			urls = append(urls, u)
		}
		return urls, nil
	}

	servers := make([]*natsserver.Server, 0, n)
	shutdownAll := func() {
		for _, ns := range servers {
			ns.Shutdown()
			ns.WaitForShutdown()
		}
	}
	for i := 0; i < n; i++ {
		urls, err := parseURLs(i)
		if err != nil {
			shutdownAll()
			return nil, err
		}
		opts := &natsserver.Options{
			Host:       "127.0.0.1",
			Port:       natsserver.RANDOM_PORT,
			JetStream:  false,
			NoLog:      true,
			NoSigs:     true,
			ServerName: fmt.Sprintf("natsbench-cluster-%d-%d", i, time.Now().UnixNano()),
			MaxPayload: 64 * 1024 * 1024,
			MaxPending: maxPending,
			Cluster: natsserver.ClusterOpts{
				Name: "natsbench-cluster",
				Host: "127.0.0.1",
				Port: ports[i],
			},
			Routes: urls,
		}
		ns, err := natsserver.NewServer(opts)
		if err != nil {
			shutdownAll()
			return nil, xerrors.Errorf("new cluster server %d: %w", i, err)
		}
		go ns.Start()
		if !ns.ReadyForConnections(15 * time.Second) {
			ns.Shutdown()
			ns.WaitForShutdown()
			shutdownAll()
			return nil, xerrors.Errorf("cluster server %d not ready", i)
		}
		servers = append(servers, ns)
	}

	// Wait for full mesh: each server should see n-1 routes.
	deadline := time.Now().Add(20 * time.Second)
	for _, ns := range servers {
		for ns.NumRoutes() < n-1 {
			if time.Now().After(deadline) {
				shutdownAll()
				return nil, xerrors.Errorf("cluster routes did not converge: %s has %d routes (want %d)",
					ns.Name(), ns.NumRoutes(), n-1)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	return servers, nil
}

// startCoderCluster brings up n coderd/x/nats.Pubsub instances in a
// full-mesh cluster, each backed by its own embedded server. Mirrors
// the cluster setup pattern in coderd/x/nats/bench_test.go. The caller
// is responsible for calling Close on each returned Pubsub.
//
// *Pubsub does not expose NumRoutes, so this function relies on a
// small sleep to allow route gossip to settle. The bench harness's
// delivery completeness check (per-subscriber target count) is what
// actually proves messages traversed routes.
// maxPending is the per-client outbound pending byte budget plumbed
// into each replica's codernats.Options. Pass 0 to use the package
// default (1 GiB); pass a positive value to override. publishConns
// and subscribeConns are the per-replica Pubsub pool sizes; the bench
// harness always pins these to benchmarkPublishConns /
// benchmarkSubscribeConns so cluster runs match standalone runs.
func startCoderCluster(ctx context.Context, logger slog.Logger, n int, maxPending int64, publishConns, subscribeConns int) ([]*codernats.Pubsub, error) {
	if n < 1 {
		return nil, xerrors.Errorf("coder cluster requires n >= 1, got %d", n)
	}
	ports := make([]int, n)
	for i := range ports {
		p, err := freeBenchPort()
		if err != nil {
			return nil, xerrors.Errorf("alloc route port %d: %w", i, err)
		}
		ports[i] = p
	}
	// Shared route auth secret used by all replicas. Not a credential;
	// the bench cluster is loopback-only and torn down at process exit.
	const token = "natsbench-coder-cluster-token" //nolint:gosec // G101: see comment

	pubsubs := make([]*codernats.Pubsub, 0, n)
	closeAll := func() {
		for _, p := range pubsubs {
			_ = p.Close()
		}
	}
	for i := 0; i < n; i++ {
		peers := make([]codernats.Peer, 0, n-1)
		for j := 0; j < n; j++ {
			if j == i {
				continue
			}
			peers = append(peers, codernats.Peer{
				Name:     fmt.Sprintf("natsbench-coder-%d", j),
				RouteURL: fmt.Sprintf("nats://127.0.0.1:%d", ports[j]),
			})
		}
		opts := codernats.Options{
			ServerName:       fmt.Sprintf("natsbench-coder-%d", i),
			ClusterName:      "natsbench-coder-cluster",
			ClusterToken:     token,
			ClusterHost:      "127.0.0.1",
			ClusterPort:      ports[i],
			ClusterAdvertise: net.JoinHostPort("127.0.0.1", strconv.Itoa(ports[i])),
			PeerProvider:     codernats.StaticPeerProvider(peers),
			ReadyTimeout:     30 * time.Second,
			MaxPending:       maxPending,
			PublishConns:     publishConns,
			SubscribeConns:   subscribeConns,
			PendingLimits: codernats.PendingLimits{
				Msgs:  -1,
				Bytes: -1,
			},
		}
		p, err := codernats.New(ctx, logger, opts)
		if err != nil {
			closeAll()
			return nil, xerrors.Errorf("coder pubsub New (cluster replica %d): %w", i, err)
		}
		pubsubs = append(pubsubs, p)
	}
	// Pubsub does not expose route counts. Give gossip a moment to
	// converge before the benchmark hot loop runs. Empirically 500ms
	// is plenty for a loopback full mesh of up to 10 replicas.
	if n > 1 {
		time.Sleep(500 * time.Millisecond)
	}
	return pubsubs, nil
}
