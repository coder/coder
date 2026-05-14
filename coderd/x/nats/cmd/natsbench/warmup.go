package main

import (
	"sync/atomic"
	"time"
)

// warmupSentinel is the first byte of a warmup payload. Hot benchmark
// payloads are filled with payload[i] = byte(i), so payload[0] == 0,
// which is distinct from warmupSentinel and lets subscriber callbacks
// route warmup messages to the warmup counter without affecting the
// exact-delivery hot counter.
const warmupSentinel byte = 0xFF

// warmupReplicaCap is the maximum cluster replica count for which the
// warmup helpers can track per-replica interest in a uint64 bitmask.
// Cluster benchmark runs exercise <= 10 replicas in practice; 64 is a
// comfortable headroom. Above this cap, the warmup phase falls back to
// "any one warmup observed" semantics and logs a note to stderr.
const warmupReplicaCap = 64

// warmupPayload builds a small warmup payload tagged with the publishing
// replica index. Length is fixed at 2 bytes; subscribers identify the
// payload via data[0] == warmupSentinel and decode data[1] as the
// publishing replica index. A constant tiny size keeps warmup traffic
// negligible compared to the hot phase even if many warmup rounds are
// needed before all routes settle.
func warmupPayload(replicaIdx int) []byte {
	if replicaIdx < 0 || replicaIdx > 255 {
		replicaIdx = 0
	}
	return []byte{warmupSentinel, byte(replicaIdx)}
}

// isWarmupPayload reports whether data was tagged by warmupPayload.
// The check is intentionally tolerant of empty payloads: a zero-length
// data slice cannot be a warmup payload (warmup payloads are 2 bytes).
func isWarmupPayload(data []byte) bool {
	return len(data) >= 1 && data[0] == warmupSentinel
}

// warmupReplicaIdx returns the encoded replica index from a warmup
// payload. Returns -1 for non-warmup payloads or warmup payloads that
// were truncated.
func warmupReplicaIdx(data []byte) int {
	if !isWarmupPayload(data) || len(data) < 2 {
		return -1
	}
	return int(data[1])
}

// expectedWarmupMask returns the bitmask of publishing-replica indices
// that should produce warmup messages on each subject, for the run
// described by plan and pubReplicaOf. pubReplicaOf(i) maps publisher i
// to its hosting replica index. Returns a slice of length plan.NumSubjects.
//
// For runners that pin all publishers to replica 0 (runCoderCluster,
// runNativeCluster), pubReplicaOf returns 0 for every i and the mask
// for each populated subject collapses to just bit 0.
//
// Subjects that have zero publishers in the plan get a zero mask
// (no warmup needed; subscribers on that subject expect zero
// deliveries anyway).
func expectedWarmupMask(plan subjectPlan, pubReplicaOf func(int) int) []uint64 {
	out := make([]uint64, plan.NumSubjects)
	for i := 0; i < len(plan.PubSubject); i++ {
		s := plan.PubSubject[i]
		r := pubReplicaOf(i)
		if r < 0 || r >= warmupReplicaCap {
			// Fall back to "any one warmup observed" semantics:
			// set bit 0 so subscribers only have to see one warmup
			// regardless of which replica it came from.
			out[s] |= 1
			continue
		}
		out[s] |= 1 << uint(r)
	}
	return out
}

// warmupState tracks which replicas a subscriber has seen warmup from.
// It is safe for concurrent use: bits are set under atomic OR.
type warmupState struct {
	seen atomic.Uint64
}

func (w *warmupState) mark(replicaIdx int) {
	if w == nil || replicaIdx < 0 || replicaIdx >= warmupReplicaCap {
		// Out-of-range index defaults to bit 0 so the warmup loop
		// still makes progress when the cluster exceeds warmupReplicaCap.
		if w != nil {
			w.seen.Or(1)
		}
		return
	}
	w.seen.Or(1 << uint(replicaIdx))
}

// satisfied reports whether seen covers expected (every bit set in
// expected is also set in seen).
func (w *warmupState) satisfied(expected uint64) bool {
	if expected == 0 {
		return true
	}
	if w == nil {
		return false
	}
	return w.seen.Load()&expected == expected
}

// reset clears the seen bitmask. Called between warmup phase and the
// hot loop in case any in-flight warmup messages are still being
// processed by the dispatcher when the hot phase begins; resetting at
// the end of warmup is a defensive measure since hot payloads are
// already filtered by isWarmupPayload, but it keeps the state easy to
// reason about post-warmup.
func (w *warmupState) reset() {
	if w == nil {
		return
	}
	w.seen.Store(0)
}

// warmupRunner is the abstract publish-side of warmup. Implementations
// know how to publish a warmup payload via the right transport and
// replica binding. The warmup loop calls publish(subject, replica)
// repeatedly until either every subscriber on every subject is
// satisfied or the deadline fires.
type warmupRunner interface {
	// publishWarmup sends a warmup payload (tagged with replica) on
	// the given subject from the given replica. Returns the first
	// transport error.
	publishWarmup(subject string, replica int) error
	// flushReplica blocks until the given replica has flushed all
	// queued warmup publishes to the server. Used to bound the
	// per-round latency without spinning.
	flushReplica(replica int) error
}

// warmupSubjectsBlocking publishes warmup messages on every subject
// until each subscriber on each subject has observed warmup from every
// expected publishing replica, or the deadline fires. expected[s] is the
// bitmask of replicas that should produce warmup on subject s (zero
// means subject s has no publishers and is skipped). subscriberSubject
// maps subscriber index -> subject index. subscribers[j].satisfied is
// consulted each round. After the loop completes (success or timeout)
// every subscriber's warmup state is reset.
//
// The pollInterval is the wait between rounds. retryRounds is a soft
// cap: if expected[s] still is not satisfied for any subscriber after
// retryRounds, the function returns nil anyway and the operator sees
// any resulting hot-phase delivery shortfall via the normal timeout
// path. This avoids the warmup phase becoming the new "silent hang"
// when a cluster route never converges.
func warmupSubjectsBlocking(
	subjects []string,
	expected []uint64,
	subscriberSubject []int,
	subscribers []*warmupState,
	pubReplicasBySubject [][]int,
	r warmupRunner,
	deadline time.Time,
	pollInterval time.Duration,
) error {
	if len(subjects) == 0 || len(expected) == 0 {
		return nil
	}
	defer func() {
		for _, s := range subscribers {
			s.reset()
		}
	}()
	for {
		// Send one warmup per (subject, replica) pair that has not yet
		// been observed by all subscribers on that subject.
		anyPending := false
		for s, mask := range expected {
			if mask == 0 {
				continue
			}
			// Identify subscribers on this subject that are not yet
			// satisfied. If all are satisfied we can skip this subject.
			needSomething := false
			for j, subj := range subscriberSubject {
				if subj != s {
					continue
				}
				if !subscribers[j].satisfied(mask) {
					needSomething = true
					break
				}
			}
			if !needSomething {
				continue
			}
			anyPending = true
			for _, r0 := range pubReplicasBySubject[s] {
				if err := r.publishWarmup(subjects[s], r0); err != nil {
					return err
				}
			}
		}
		// Flush every replica that participates. Without flushing, the
		// warmup messages can sit in the per-conn write buffer and the
		// loop spins until the buffer auto-flushes.
		flushed := make(map[int]struct{})
		for s, mask := range expected {
			if mask == 0 {
				continue
			}
			_ = s
			for _, r0 := range pubReplicasBySubject[s] {
				if _, ok := flushed[r0]; ok {
					continue
				}
				flushed[r0] = struct{}{}
				if err := r.flushReplica(r0); err != nil {
					return err
				}
			}
		}
		if !anyPending {
			return nil
		}
		// Check if everyone is now satisfied. If so, we are done.
		allSat := true
		for j, s := range subscriberSubject {
			if !subscribers[j].satisfied(expected[s]) {
				allSat = false
				break
			}
		}
		if allSat {
			return nil
		}
		if !time.Now().Before(deadline) {
			// Soft cap: return nil so the hot phase still runs. The
			// hot-phase timeout will surface any actual interest
			// propagation failure as a delivery shortfall.
			return nil
		}
		time.Sleep(pollInterval)
	}
}

// pubReplicasPerSubject groups publishers by their assigned subject
// and returns, for each subject, the sorted, deduplicated set of
// publishing-replica indices.
func pubReplicasPerSubject(plan subjectPlan, pubReplicaOf func(int) int) [][]int {
	out := make([][]int, plan.NumSubjects)
	for s := range out {
		out[s] = nil
	}
	for i, s := range plan.PubSubject {
		r := pubReplicaOf(i)
		// Deduplicate.
		dup := false
		for _, existing := range out[s] {
			if existing == r {
				dup = true
				break
			}
		}
		if !dup {
			out[s] = append(out[s], r)
		}
	}
	return out
}
