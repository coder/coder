package main

import (
	"fmt"
	"math/rand/v2"
	"slices"
)

// plan assigns each publisher and subscriber to a subject round-robin by
// index (index % Subjects), places each one on a pseudorandom replica
// node (seeded for reproducibility), and precomputes each subscriber's
// expected delivery count so the workload can do exact accounting.
//
// Node placement is random to model a real deployment, where each
// client connects to an arbitrary replica. Round-robin node placement
// would instead co-locate publisher i and subscriber i on the same
// node and, whenever Replicas divides Subjects, keep a subject's entire
// traffic local, which understates cross-node routing cost.
//
// Worked example with Subjects=2, Publishers=3, Subscribers=4,
// Messages=100. Subject assignment is round-robin (index % Subjects)
// and the 100 messages split 34/33/33 (the remainder lands on publisher
// 0):
//
//	publisher 0 -> subject 0, sends 34
//	publisher 1 -> subject 1, sends 33
//	publisher 2 -> subject 0, sends 33
//
//	subject 0 receives 34+33 = 67 (publishers 0 and 2)
//	subject 1 receives 33      (publisher 1)
//
//	subscriber 0 -> subject 0, expects 67
//	subscriber 1 -> subject 1, expects 33
//	subscriber 2 -> subject 0, expects 67
//	subscriber 3 -> subject 1, expects 33
//
// totalExpected = 67+33+67+33 = 200. It exceeds the 100 published
// messages because each subject has two subscribers, so every message
// is delivered twice. That fan-out is the logical delivery count the
// benchmark measures. Each publisher's and subscriber's node is drawn
// independently from [0, Replicas).
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

// buildPlan computes the workload assignment for cfg. Subject and
// message assignment is deterministic; node placement is pseudorandom
// but fully determined by cfg.Seed, so a given config reproduces the
// same plan. cfg must already be validated: all counts are at least 1.
func buildPlan(cfg Config) plan {
	pl := plan{
		perPubMsgs:   make([]int, cfg.Publishers),
		pubSubject:   make([]int, cfg.Publishers),
		pubNode:      make([]int, cfg.Publishers),
		subSubject:   make([]int, cfg.Subscribers),
		subNode:      make([]int, cfg.Subscribers),
		expectPerSub: make([]int, cfg.Subscribers),
	}

	// Seed both PCG streams from cfg.Seed so placement is reproducible
	// for a given seed. Node placement is benchmark randomness, not
	// security-sensitive.
	seed := uint64(cfg.Seed)                 //nolint:gosec // G115: deterministic benchmark seed, sign irrelevant.
	rng := rand.New(rand.NewPCG(seed, seed)) //nolint:gosec // G404: placement randomness, not security-sensitive.

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
		pl.pubNode[i] = rng.IntN(cfg.Replicas)
		perSubjectMsgs[pl.pubSubject[i]] += msgs
	}

	for j := range cfg.Subscribers {
		pl.subSubject[j] = j % cfg.Subjects
		pl.subNode[j] = rng.IntN(cfg.Replicas)
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
