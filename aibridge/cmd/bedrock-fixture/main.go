// Command bedrock-fixture decodes a captured AWS Bedrock EventStream
// response (.resp.bin) into the human-readable JSON form used inside
// Bedrock txtar fixtures.
//
// Usage:
//
//	go run ./aibridge/cmd/bedrock-fixture <path-to-resp.bin>
//
// The decoded JSON is written to stdout. Compose with the request and
// (optional) blocking response JSON files to assemble a final fixture:
//
//	{
//	  echo "-- request --"
//	  cat simple.req.json
//	  echo
//	  echo "-- streaming --"
//	  go run ./aibridge/cmd/bedrock-fixture simple_streaming.resp.bin
//	  echo
//	  echo "-- blocking --"
//	  cat simple_blocking.resp.json
//	} > aibridge/fixtures/bedrock/simple.txtar
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/coder/coder/v2/aibridge/fixtures"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: bedrock-fixture <path-to-resp.bin>")
		os.Exit(2)
	}

	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	frames, err := fixtures.DecodeAWSEventStream(data)
	if err != nil {
		log.Fatal(err)
	}

	out, err := json.MarshalIndent(frames, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stdout.Write(out); err != nil {
		log.Fatal(err)
	}
	fmt.Println()
}
