package natsbench

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/xerrors"
)

const (
	// probeSentinel distinguishes readiness probes from benchmark
	// payloads. Benchmark payloads are all zeros, so a payload whose
	// first byte is the sentinel can only be a probe.
	probeSentinel byte = 0x5b
	// probeLen is the probe payload size: one sentinel byte plus a
	// BigEndian uint64 publisher-node index.
	probeLen = 9
	// probeInterval is how often probes are re-published while waiting
	// for cross-route subscription interest to propagate.
	probeInterval = 25 * time.Millisecond
)

// probePayload encodes a readiness probe identifying the publishing
// node.
func probePayload(node int) []byte {
	payload := make([]byte, probeLen)
	payload[0] = probeSentinel
	// #nosec G115 - node is a replica index, always small and non-negative.
	binary.BigEndian.PutUint64(payload[1:], uint64(node))
	return payload
}

// probeNode decodes a readiness probe. It reports false for benchmark
// payloads.
func probeNode(payload []byte) (int, bool) {
	if len(payload) != probeLen || payload[0] != probeSentinel {
		return 0, false
	}
	raw := binary.BigEndian.Uint64(payload[1:])
	// Replica indexes are tiny; anything larger is not a probe.
	if raw > math.MaxInt32 {
		return 0, false
	}
	return int(raw), true
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

// publisherNodeSubjects returns, per publisher node, the sorted subject
// indexes that node publishes to.
func publisherNodeSubjects(pl plan) map[int][]int {
	sets := make(map[int]map[int]struct{})
	for i, node := range pl.pubNode {
		if sets[node] == nil {
			sets[node] = make(map[int]struct{})
		}
		sets[node][pl.pubSubject[i]] = struct{}{}
	}
	out := make(map[int][]int, len(sets))
	for node, subjects := range sets {
		list := make([]int, 0, len(subjects))
		for subject := range subjects {
			list = append(list, subject)
		}
		slices.Sort(list)
		out[node] = list
	}
	return out
}

// requiredNodesPerSubscriber returns, per subscriber, the set of
// publisher nodes that target its subject. A subscriber whose subject
// has no publishers requires nothing.
func requiredNodesPerSubscriber(pl plan) []map[int]struct{} {
	subjectNodes := make(map[int]map[int]struct{})
	for i, node := range pl.pubNode {
		subject := pl.pubSubject[i]
		if subjectNodes[subject] == nil {
			subjectNodes[subject] = make(map[int]struct{})
		}
		subjectNodes[subject][node] = struct{}{}
	}
	out := make([]map[int]struct{}, len(pl.subSubject))
	for j, subject := range pl.subSubject {
		out[j] = subjectNodes[subject]
	}
	return out
}

// awaitReadiness proves that subscription interest has propagated to
// every publisher node before the measured phase. Each publisher node
// repeatedly publishes an in-band probe on every subject it will
// publish to; the gate converges when every subscriber has observed a
// probe from every publisher node targeting its subject. Without this,
// routed deliveries silently undercount on fresh clusters.
func awaitReadiness(ctx context.Context, top *topology, pl plan, timeout time.Duration, trackers []*probeTracker) error {
	nodeSubjects := publisherNodeSubjects(pl)
	required := requiredNodesPerSubscriber(pl)

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(probeInterval)
	defer ticker.Stop()

	for {
		for node, subjects := range nodeSubjects {
			for _, subject := range subjects {
				if err := top.nodes[node].Publish(subjectName(subject), probePayload(node)); err != nil {
					return xerrors.Errorf("publish probe from node %d on %s: %w", node, subjectName(subject), err)
				}
			}
			if err := top.nodes[node].Flush(); err != nil {
				return xerrors.Errorf("flush probes from node %d: %w", node, err)
			}
		}

		if readinessConverged(trackers, required) {
			return nil
		}

		select {
		case <-ctx.Done():
			return xerrors.Errorf("readiness gate canceled: %w", ctx.Err())
		case <-deadline.C:
			return xerrors.Errorf("readiness gate timed out after %s: %s", timeout, readinessShortfall(trackers, required))
		case <-ticker.C:
		}
	}
}

func readinessConverged(trackers []*probeTracker, required []map[int]struct{}) bool {
	for j, tracker := range trackers {
		if len(tracker.missing(required[j])) > 0 {
			return false
		}
	}
	return true
}

// readinessShortfall describes which subscribers are still missing
// probes from which publisher nodes.
func readinessShortfall(trackers []*probeTracker, required []map[int]struct{}) string {
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
