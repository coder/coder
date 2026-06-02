package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	for _, e := range os.Environ() {
		upper := strings.ToUpper(e)
		if strings.HasPrefix(upper, "PATH=") {
			// Print key=first_100_chars_of_value
			key, val, _ := strings.Cut(e, "=")
			if len(val) > 200 {
				val = val[:200] + "..."
			}
			_, _ = fmt.Printf("Go sees: %s=%s\n", key, val)
		}
	}
}
