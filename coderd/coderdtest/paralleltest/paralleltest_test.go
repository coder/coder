package paralleltest

import (
	"strconv"
	"testing"
	"time"

	_ "github.com/coder/coder/v2/coderd/coderdtest"
)

var count = 0

func TestParallelism(t *testing.T) {
	t.Parallel()

	for i := 0; i < 5; i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			count++
			time.Sleep(time.Second)
		})
	}
}
