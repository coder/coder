package natsbench

import "fmt"

// Scenario names one benchmark configuration in the standard matrix.
type Scenario struct {
	Name   string
	Config Config
}

// DefaultScenarios returns the standard matrix: payloads of 8 KiB and
// 64 KiB at 1, 5, and 10 replicas. The 64 KiB cluster runs use a
// reduced total message count because the fan-out byte volume is
// memory-heavy.
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
					Messages:    messages,
					PayloadSize: payload.size,
					Subjects:    subjects,
					Publishers:  publishers,
					Subscribers: subscribers,
					Replicas:    replicas,
				},
			})
		}
	}
	return scenarios
}
