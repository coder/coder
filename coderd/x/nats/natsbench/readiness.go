package main

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/xerrors"
)

const (
	// probePrefix tags readiness probes so they are distinguishable
	// from benchmark payloads. Benchmark payloads are all zeros, so any
	// payload starting with this non-zero ASCII prefix can only be a
	// probe.
	probePrefix = "natsbench-probe:"
	// probeInterval is how often probes are re-published while waiting
	// for cross-route subscription interest to propagate.
	probeInterval = 25 * time.Millisecond
)

// probePrefixBytes avoids per-call allocation in probeNode, which runs
// on every delivered message.
var probePrefixBytes = []byte(probePrefix)

// probePayload encodes a readiness probe identifying the publishing
// node: the probe prefix followed by the node index in decimal ASCII.
func probePayload(node int) []byte {
	return []byte(probePrefix + strconv.Itoa(node))
}

// probeNode decodes a readiness probe. It reports false for benchmark
// payloads, which are all zeros and never match the probe prefix.
func probeNode(payload []byte) (int, bool) {
	rest, ok := bytes.CutPrefix(payload, probePrefixBytes)
	if !ok {
		return 0, false
	}
	node, err := strconv.Atoi(string(rest))
	if err != nil {
		return 0, false
	}
	return node, true
}

// probeTracker records which publisher nodes a subscriber has observed
// probes from.
type probeTracker struct {
	mu   sync.Mutex
	seen map[int]struct{}
}

func newProbeTracker() *probeTracker {
	return &probeTracker{seen: make(map[int]struct{})}
}

func (t *probeTracker) observe(node int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.seen[node] = struct{}{}
}

// missing returns the required node indexes not yet observed, sorted.
func (t *probeTracker) missing(required map[int]struct{}) []int {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []int
	for node := range required {
		if _, ok := t.seen[node]; !ok {
			out = append(out, node)
		}
	}
	slices.Sort(out)
	return out
}

// subjectNodes maps each subject index to the set of publisher node
// indexes that publish to it. This single mapping drives the whole
// gate: it is both the probe schedule (each node probes its subjects)
// and, looked up by a subscriber's subject, that subscriber's required
// probe set. Subjects without publishers are absent (nothing required).
func subjectNodes(pl plan) map[int]map[int]struct{} {
	out := make(map[int]map[int]struct{})
	for i, node := range pl.pubNode {
		subject := pl.pubSubject[i]
		if out[subject] == nil {
			out[subject] = make(map[int]struct{})
		}
		out[subject][node] = struct{}{}
	}
	return out
}

// readinessCheckInterval is how often the gate polls for convergence.
// It is finer than probeInterval so the measured convergence time
// reflects when probes actually propagated rather than the probe
// republish cadence.
const readinessCheckInterval = time.Millisecond

// awaitTopologyReady proves that subscription interest has propagated to
// every publisher node before the measured phase, and returns how long
// that took. Each publisher node repeatedly publishes an in-band probe
// on every subject it will publish to; the gate converges when every
// subscriber has observed a probe from every publisher node targeting
// its subject. Without this, routed deliveries silently undercount on
// fresh clusters.
//
// The returned duration is the cluster convergence time: from the first
// probe to the moment interest has propagated everywhere. It is
// measured from gate entry, which is immediately after all subscriptions
// are registered.
func awaitTopologyReady(ctx context.Context, top *topology, pl plan, timeout time.Duration, trackers []*probeTracker) (time.Duration, error) {
	bySubject := subjectNodes(pl)
	required := make([]map[int]struct{}, len(pl.subSubject))
	for j, subject := range pl.subSubject {
		required[j] = bySubject[subject]
	}

	publishProbes := func() error {
		for subject, nodes := range bySubject {
			for node := range nodes {
				if err := top.nodes[node].Publish(subjectName(subject), probePayload(node)); err != nil {
					return xerrors.Errorf("publish probe from node %d on %s: %w", node, subjectName(subject), err)
				}
			}
		}
		for _, node := range pl.pubNodes {
			if err := top.nodes[node].Flush(); err != nil {
				return xerrors.Errorf("flush probes from node %d: %w", node, err)
			}
		}
		return nil
	}

	started := time.Now()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	publishTicker := time.NewTicker(probeInterval)
	defer publishTicker.Stop()
	checkTicker := time.NewTicker(readinessCheckInterval)
	defer checkTicker.Stop()

	if err := publishProbes(); err != nil {
		return 0, err
	}
	if isReady(trackers, required) {
		return time.Since(started), nil
	}

	for {
		select {
		case <-ctx.Done():
			return 0, xerrors.Errorf("readiness gate canceled: %w", ctx.Err())
		case <-deadline.C:
			return 0, xerrors.Errorf("readiness gate timed out after %s: %s", timeout, unreadySubscribers(trackers, required))
		case <-publishTicker.C:
			if err := publishProbes(); err != nil {
				return 0, err
			}
		case <-checkTicker.C:
			if isReady(trackers, required) {
				return time.Since(started), nil
			}
		}
	}
}

func isReady(trackers []*probeTracker, required []map[int]struct{}) bool {
	for j, tracker := range trackers {
		if len(tracker.missing(required[j])) > 0 {
			return false
		}
	}
	return true
}

// unreadySubscribers describes which subscribers are still missing
// probes from which publisher nodes.
func unreadySubscribers(trackers []*probeTracker, required []map[int]struct{}) string {
	const maxEntries = 20
	var entries []string
	for j, tracker := range trackers {
		missing := tracker.missing(required[j])
		if len(missing) == 0 {
			continue
		}
		if len(entries) >= maxEntries {
			entries = append(entries, "...")
			break
		}
		entries = append(entries, fmt.Sprintf("subscriber %d missing probes from nodes %v", j, missing))
	}
	return strings.Join(entries, "; ")
}
