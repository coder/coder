package coderdtest

import (
	"flag"
	"testing"
)

func init() {
	testing.Init()
	flag.Set("test.parallel", "1") // Disable test parallelism
}
