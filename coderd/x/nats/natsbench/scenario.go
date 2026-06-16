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

// DefaultScenarios returns the standard matrix: payloads of 8 KiB and
// 64 KiB at 1, 3, and 9 replicas. The 64 KiB cluster runs use a reduced
// total message count because the fan-out byte volume is memory-heavy.
//
// The replica counts are deliberately coprime with the subject count
// (10): the round-robin placement co-locates every publisher and
// subscriber on a subject whenever Subjects % Replicas == 0, so divisor
// replica counts would never exercise cross-node routing. 3 and 9 force
// cross-node pairs, making the readiness gate prove route propagation
// and the throughput numbers include routing cost.
//
// The returned configs leave Timeout unset; callers must set it before
// passing them to Run (the CLI does this from its -timeout flag).
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
		for _, replicas := range []int{1, 3, 9} {
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
