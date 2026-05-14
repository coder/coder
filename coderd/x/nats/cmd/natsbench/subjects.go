package main

import (
	"fmt"

	"golang.org/x/xerrors"
)

// Benchmark-only pool sizes pinned for Coder modes. We hardcode these
// instead of exposing a CLI flag because the only pool size we want to
// measure right now is the default NATS route pool size of 3; sweeping
// pool sizes is out of scope for this iteration. These constants are
// applied to every Coder Pubsub instance constructed by natsbench
// (coder-tcp, coder-inproc, coder-cluster, coder-cluster-symmetric) so
// runs are directly comparable across modes.
const (
	benchmarkPublishConns   = 3
	benchmarkSubscribeConns = 3
)

// subjectPlan describes the deterministic distribution of publishers and
// subscribers across a fixed set of subjects, plus the expected
// per-subscriber delivery count. The plan is shared by every natsbench
// mode so Coder and native runs are directly comparable.
//
// Distribution shape (all modes):
//
//   - There are NumSubjects subjects, indexed [0, NumSubjects).
//   - PubSubject[i] = i mod NumSubjects: publisher i is pinned to one
//     subject for the entire run.
//   - SubSubject[j] = j mod NumSubjects: subscriber j is pinned to one
//     subject for the entire run and expects only messages published to
//     that subject.
//   - PerPubMsgs[i] is the message count publisher i emits.
//   - ExpectPerSub[j] is the message count subscriber j should receive:
//     the sum of PerPubMsgs[i] over every publisher i whose subject
//     equals subscriber j's subject.
//
// With NumSubjects == 1 the plan collapses to "every pub and every sub
// uses subject 0" and PerPubMsgs / ExpectPerSub reproduce the legacy
// single-subject behavior exactly.
type subjectPlan struct {
	NumSubjects    int
	PubSubject     []int
	SubSubject     []int
	PerPubMsgs     []int
	ExpectPerSub   []int64
	TotalPublished int64
}

// planSubjects builds a subjectPlan for the given run shape.
//
//   - pubs, subs are the number of publisher and subscriber goroutines.
//   - numSubjects is the number of subjects, must be >= 1.
//   - msgs is the publish budget. When perPub is false, msgs is the
//     total publish budget shared by all publishers (split as evenly as
//     possible with any remainder dumped on publisher 0, matching the
//     historical natsbench split). When perPub is true (symmetric
//     cluster modes), each publisher emits exactly msgs messages.
//
// planSubjects returns an error only for invalid shape inputs; counts
// of 0 publishers or 0 subscribers are valid and produce empty slices.
//
//nolint:revive // perPub selects how msgs is split, not a control flag.
func planSubjects(pubs, subs, numSubjects, msgs int, perPub bool) (subjectPlan, error) {
	if numSubjects < 1 {
		return subjectPlan{}, xerrors.Errorf("subjects must be >= 1, got %d", numSubjects)
	}
	if pubs < 0 || subs < 0 || msgs < 0 {
		return subjectPlan{}, xerrors.Errorf("pubs, subs, msgs must be >= 0, got pubs=%d subs=%d msgs=%d", pubs, subs, msgs)
	}

	plan := subjectPlan{
		NumSubjects:  numSubjects,
		PubSubject:   make([]int, pubs),
		SubSubject:   make([]int, subs),
		PerPubMsgs:   make([]int, pubs),
		ExpectPerSub: make([]int64, subs),
	}

	// PerPubMsgs split.
	if pubs > 0 {
		if perPub {
			for i := 0; i < pubs; i++ {
				plan.PerPubMsgs[i] = msgs
			}
		} else {
			perPubBase, rem := msgs/pubs, msgs%pubs
			for i := 0; i < pubs; i++ {
				n := perPubBase
				if i == 0 {
					n += rem
				}
				plan.PerPubMsgs[i] = n
			}
		}
	}

	// Subject assignment + per-subject published total.
	publishedPerSubject := make([]int64, numSubjects)
	for i := 0; i < pubs; i++ {
		s := i % numSubjects
		plan.PubSubject[i] = s
		publishedPerSubject[s] += int64(plan.PerPubMsgs[i])
		plan.TotalPublished += int64(plan.PerPubMsgs[i])
	}

	// Per-subscriber expected count.
	for j := 0; j < subs; j++ {
		s := j % numSubjects
		plan.SubSubject[j] = s
		plan.ExpectPerSub[j] = publishedPerSubject[s]
	}

	return plan, nil
}

// nativeSubject returns the i'th native NATS subject for the run. When
// total == 1 the prefix is used as-is (preserving the legacy single
// subject "bench" behavior). When total > 1 the form is "<prefix>.<i>"
// so each subject is a distinct token under the same domain.
func nativeSubject(prefix string, i, total int) string {
	if total <= 1 {
		return prefix
	}
	return fmt.Sprintf("%s.%d", prefix, i)
}

// coderSubject returns the i'th legacy event name for the run. Tokens
// must satisfy coderd/x/nats.ValidateToken, so dots in the prefix would
// produce an invalid event. When total == 1 the prefix is used as-is
// (the caller is responsible for choosing a token-valid prefix; the
// default "bench" qualifies). When total > 1 the form is
// "<prefix>_<i>", which only uses underscore as a separator and stays
// within the [A-Za-z0-9_-] token alphabet enforced by
// codernats.LegacyEventSubject.
func coderSubject(prefix string, i, total int) string {
	if total <= 1 {
		return prefix
	}
	return fmt.Sprintf("%s_%d", prefix, i)
}

// buildNativeSubjects returns a slice of length total filled with
// nativeSubject(prefix, i, total).
func buildNativeSubjects(prefix string, total int) []string {
	out := make([]string, total)
	for i := 0; i < total; i++ {
		out[i] = nativeSubject(prefix, i, total)
	}
	return out
}

// buildCoderSubjects returns a slice of length total filled with
// coderSubject(prefix, i, total).
func buildCoderSubjects(prefix string, total int) []string {
	out := make([]string, total)
	for i := 0; i < total; i++ {
		out[i] = coderSubject(prefix, i, total)
	}
	return out
}
