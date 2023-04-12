package testutil

import (
	"flag"
	"os"
)

func InCI() bool {
	_, ok := os.LookupEnv("CI")
	return ok
}

func InRaceMode() bool {
	fl := flag.Lookup("race")
	return fl != nil && fl.Value.String() == "true"
}
