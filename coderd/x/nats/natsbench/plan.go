package natsbench

import "fmt"

// plan deterministically maps publishers and subscribers onto subjects
// and replica nodes, and precomputes each subscriber's expected
// delivery count so the workload can do exact accounting.
type plan struct {
	// perPubMsgs[i] is the number of messages publisher i sends. The
	// configured total is split evenly across publishers with the
	// remainder assigned to publisher 0.
	perPubMsgs []int
	// pubSubject[i] and pubNode[i] are publisher i's subject and node
	// indexes: subject i % Subjects on node i % Replicas.
	pubSubject []int
	pubNode    []int
	// subSubject[j] and subNode[j] are subscriber j's subject and node
	// indexes: subject j % Subjects on node j % Replicas.
	subSubject []int
	subNode    []int
	// expectPerSub[j] is the number of benchmark messages subscriber j
	// must observe: the sum of perPubMsgs over publishers sharing its
	// subject.
	expectPerSub []int
	// totalExpected is the sum of expectPerSub, the logical delivery
	// count. Fan-out makes it exceed the publish total whenever a
	// subject has more than one subscriber.
	totalExpected int
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
	return pl
}

// subjectName returns the NATS subject for subject index i.
func subjectName(i int) string {
	return fmt.Sprintf("bench.%d", i)
}
