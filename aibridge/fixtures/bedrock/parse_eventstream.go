//go:build ignore

// Usage:
//
//	go run parse_eventstream.go [dir]
//
// Finds all .resp.bin files in dir (default: current directory) and
// generates a corresponding .resp.decoded file for each one. Existing
// .resp.decoded files are overwritten.
//
// To decode a single file:
//
//	go run parse_eventstream.go path/to/file.resp.bin
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream/eventstreamapi"
)

func main() {
	arg := "."
	if len(os.Args) > 1 {
		arg = os.Args[1]
	}

	info, err := os.Stat(arg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stat %s: %v\n", arg, err)
		os.Exit(1)
	}

	if !info.IsDir() {
		if err := decodeFile(arg, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		return
	}

	files, err := filepath.Glob(filepath.Join(arg, "*.resp.bin"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "glob: %v\n", err)
		os.Exit(1)
	}
	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "no .resp.bin files found in %s\n", arg)
		os.Exit(1)
	}

	for _, binFile := range files {
		outFile := strings.TrimSuffix(binFile, ".resp.bin") + ".resp.decoded"
		f, err := os.Create(outFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "create %s: %v\n", outFile, err)
			continue
		}
		if err := decodeFile(binFile, f); err != nil {
			fmt.Fprintf(os.Stderr, "decode %s: %v\n", binFile, err)
		}
		f.Close()
		fmt.Printf("%s -> %s\n", filepath.Base(binFile), filepath.Base(outFile))
	}
}

func decodeFile(path string, w *os.File) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	decoder := eventstream.NewDecoder()
	reader := bytes.NewReader(data)
	frameNum := 0

	for {
		msg, err := decoder.Decode(reader, nil)
		if err != nil {
			break
		}
		frameNum++

		messageType := msg.Headers.Get(eventstreamapi.MessageTypeHeader)
		eventType := msg.Headers.Get(eventstreamapi.EventTypeHeader)

		fmt.Fprintf(w, "=== Frame %d ===\n", frameNum)
		fmt.Fprintf(w, "  message-type: %s\n", headerStr(messageType))
		fmt.Fprintf(w, "  event-type:   %s\n", headerStr(eventType))

		if headerStr(eventType) != "chunk" {
			fmt.Fprintf(w, "  payload: %s\n\n", string(msg.Payload))
			continue
		}

		var chunk struct {
			Bytes string `json:"bytes"`
		}
		if err := json.Unmarshal(msg.Payload, &chunk); err != nil {
			fmt.Fprintf(w, "  unmarshal error: %v\n\n", err)
			continue
		}

		decoded, err := base64.StdEncoding.DecodeString(chunk.Bytes)
		if err != nil {
			fmt.Fprintf(w, "  base64 decode error: %v\n\n", err)
			continue
		}

		var pretty json.RawMessage
		if err := json.Unmarshal(decoded, &pretty); err != nil {
			fmt.Fprintf(w, "  json: %s\n\n", string(decoded))
			continue
		}

		indented, _ := json.MarshalIndent(pretty, "  ", "  ")
		fmt.Fprintf(w, "  body:\n  %s\n\n", string(indented))
	}

	fmt.Fprintf(w, "Total frames: %d\n", frameNum)
	return nil
}

func headerStr(h eventstream.Value) string {
	if h == nil {
		return "<nil>"
	}
	return h.String()
}
