package testutil

import (
	"os"
)

func InCI() bool {
	_, ok := os.LookupEnv("CI")
	return ok
}
