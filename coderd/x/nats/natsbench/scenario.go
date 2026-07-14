package main

import "fmt"

// Scenario names one benchmark configuration in the standard matrix.
type Scenario struct {
	Name   string
	Config Config
}

// DefaultConns is the default size of the publisher and subscriber
// connection pools in the standard matrix. Production ships a single
// connection each, but the benchmarks use three to match the prior
// natsbench harness and to exercise cross-subject parallelism.
const DefaultConns = 3

// DefaultSeed seeds the pseudorandom node placement. With the matrix
// shape (10 publishers) and 10 replicas, most seeds leave several nodes
// publisher-less (only ~1 in 2750 seeds covers every node), which would
// skew cross-node routing measurements. 2068 was selected because it
// spreads publishers perfectly evenly across nodes at every matrix
// replica count (1, 5, and 10): one publisher per node at 10 replicas,
// two per node at 5, as TestDefaultSeedSpreadsEvenly verifies. Override
// with -seed to sample a different, more uneven (and arguably more
// realistic) placement.
const DefaultSeed int64 = 2068

// DefaultScenarios returns the standard matrix: payloads of 8 KiB and
// 64 KiB at 1, 5, and 10 replicas. The 64 KiB cluster runs use a
// reduced total message count because the fan-out byte volume is
// memory-heavy.
// Random node placement (see buildPlan) makes the multi-replica runs
// exercise cross-node routing.
//
// The returned configs leave Timeout (and Seed) unset; callers set them
// before passing the configs to Run (the CLI does this from its
// -timeout and -seed flags).
func DefaultScenarios() []Scenario {
	const (
		subjects        = 10
		publishers      = 10
		subscribers     = 50
		reducedMessages = 20000
	)
	payloads := []struct {
		label string
		size  int
	}{
		{"8KiB", Payload8KB},
		{"64KiB", Payload64KB},
	}

	var scenarios []Scenario
	for _, payload := range payloads {
		for _, replicas := range []int{1, 5, 10} {
			messages := DefaultMessages
			if payload.size == Payload64KB && replicas > 1 {
				messages = reducedMessages
			}
			scenarios = append(scenarios, Scenario{
				Name: fmt.Sprintf("%s-%dr", payload.label, replicas),
				Config: Config{
					Messages:       messages,
					PayloadSize:    payload.size,
					Subjects:       subjects,
					Publishers:     publishers,
					Subscribers:    subscribers,
					Replicas:       replicas,
					PublishConns:   DefaultConns,
					SubscribeConns: DefaultConns,
				},
			})
		}
	}
	return scenarios
}
