package main

import (
	"fmt"
	"slices"
)

// plan deterministically assigns publishers and subscribers to subjects
// and replica nodes by round-robin on their index, and precomputes each
// subscriber's expected delivery count so the workload can do exact
// accounting.
//
// Worked example with Subjects=2, Replicas=2, Publishers=3,
// Subscribers=4, Messages=100. Publisher i and subscriber j wrap around
// the subjects and nodes by index (i%2, j%2), and the 100 messages
// split 34/33/33 (the remainder lands on publisher 0):
//
//	publisher 0 -> subject 0, node 0, sends 34
//	publisher 1 -> subject 1, node 1, sends 33
//	publisher 2 -> subject 0, node 0, sends 33
//
//	subject 0 receives 34+33 = 67 (publishers 0 and 2)
//	subject 1 receives 33      (publisher 1)
//
//	subscriber 0 -> subject 0, node 0, expects 67
//	subscriber 1 -> subject 1, node 1, expects 33
//	subscriber 2 -> subject 0, node 0, expects 67
//	subscriber 3 -> subject 1, node 1, expects 33
//
// totalExpected = 67+33+67+33 = 200. It exceeds the 100 published
// messages because each subject has two subscribers, so every message
// is delivered twice. That fan-out is the logical delivery count the
// benchmark measures.
type plan struct {
	// perPubMsgs[i] is the number of messages publisher i sends.
	perPubMsgs []int
	// pubSubject[i] is publisher i's subject index.
	pubSubject []int
	// pubNode[i] is publisher i's node index.
	pubNode []int
	// subSubject[j] is subscriber j's subject index.
	subSubject []int
	// subNode[j] is subscriber j's node index.
	subNode []int
	// expectPerSub[j] is how many benchmark messages subscriber j must
	// observe: the total sent to its subject.
	expectPerSub []int
	// totalExpected is the sum of expectPerSub.
	totalExpected int
	// pubNodes and subNodes are the sorted distinct node indexes that
	// host at least one publisher or subscriber. Several publishers or
	// subscribers can share a node, so these dedupe pubNode / subNode
	// for callers that act once per node (for example flushing).
	pubNodes []int
	subNodes []int
}

// buildPlan computes the deterministic workload assignment for cfg.
// cfg must already be validated: all counts are at least 1.
func buildPlan(cfg Config) plan {
	pl := plan{
		perPubMsgs:   make([]int, cfg.Publishers),
		pubSubject:   make([]int, cfg.Publishers),
		pubNode:      make([]int, cfg.Publishers),
		subSubject:   make([]int, cfg.Subscribers),
		subNode:      make([]int, cfg.Subscribers),
		expectPerSub: make([]int, cfg.Subscribers),
	}

	base := cfg.Messages / cfg.Publishers
	remainder := cfg.Messages % cfg.Publishers
	perSubjectMsgs := make([]int, cfg.Subjects)
	for i := range cfg.Publishers {
		msgs := base
		if i == 0 {
			msgs += remainder
		}
		pl.perPubMsgs[i] = msgs
		pl.pubSubject[i] = i % cfg.Subjects
		pl.pubNode[i] = i % cfg.Replicas
		perSubjectMsgs[pl.pubSubject[i]] += msgs
	}

	for j := range cfg.Subscribers {
		pl.subSubject[j] = j % cfg.Subjects
		pl.subNode[j] = j % cfg.Replicas
		pl.expectPerSub[j] = perSubjectMsgs[pl.subSubject[j]]
		pl.totalExpected += pl.expectPerSub[j]
	}

	pl.pubNodes = uniqueInts(pl.pubNode)
	pl.subNodes = uniqueInts(pl.subNode)
	return pl
}

// uniqueInts returns the sorted distinct values of a slice.
func uniqueInts(values []int) []int {
	out := slices.Clone(values)
	slices.Sort(out)
	return slices.Compact(out)
}

// subjectName returns the NATS subject for subject index i.
func subjectName(i int) string {
	return fmt.Sprintf("bench.%d", i)
}
