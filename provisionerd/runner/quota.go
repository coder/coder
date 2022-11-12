package runner

import "github.com/coder/coder/provisionersdk/proto"

func sumCost(resources []*proto.Resource) int {
	var sum int
	for _, r := range resources {
		sum += int(r.Cost)
	}
	return sum
}
