// Package main wraps swag init with Strict mode enabled.
//
// The upstream swag CLI (v1.16.2) does not expose a --strict
// flag, so warnings about duplicate routes are silently
// ignored. This wrapper calls the Go API directly with
// Strict: true, turning those warnings into hard errors.
package main

import (
	"log"
	"os"

	"github.com/swaggo/swag/gen"
)

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags)

	outputDir := "./coderd/apidoc"
	if d := os.Getenv("SWAG_OUTPUT_DIR"); d != "" {
		outputDir = d
	}

	err := gen.New().Build(&gen.Config{
		SearchDir:          "./coderd,./codersdk,./enterprise/coderd,./enterprise/wsproxy/wsproxysdk",
		MainAPIFile:        "coderd.go",
		OutputDir:          outputDir,
		OutputTypes:        []string{"go", "json"},
		PackageName:        "apidoc",
		ParseDependency:    1,
		Strict:             true,
		OverridesFile:      gen.DefaultOverridesFile,
		ParseGoList:        true,
		ParseDepth:         100,
		CollectionFormat:   "csv",
		Debugger:           logger,
		LeftTemplateDelim:  "{{",
		RightTemplateDelim: "}}",
	})
	if err != nil {
		log.Fatalf("swag init failed: %v", err)
	}
}
