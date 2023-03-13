package main

import (
	"os"
	"strings"

	"github.com/coder/coder/enterprise/cli"
	"github.com/coder/flog"
)

// route is an individual page object in the docs manifest.json.
type route struct {
	Title       string  `json:"title,omitempty"`
	Description string  `json:"description,omitempty"`
	Path        string  `json:"path,omitempty"`
	IconPath    string  `json:"icon_path,omitempty"`
	State       string  `json:"state,omitempty"`
	Children    []route `json:"children,omitempty"`
}

// manifest describes the entire documentation index.
type manifest struct {
	Versions []string `json:"versions,omitempty"`
	Routes   []route  `json:"routes,omitempty"`
}

func unsetCoderEnv() {
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "CODER_") {
			split := strings.SplitN(env, "=", 2)
			if err := os.Unsetenv(split[0]); err != nil {
				flog.Fatalf("Unable to unset %v: %v", split[0], err)
			}
		}
	}
}

func main() {
	unsetCoderEnv()

	workdir, err := os.Getwd()
	if err != nil {
		flog.Fatalf("getwd: %v", err)
	}
	root := (&cli.RootCmd{})

	wroteMap := make(map[string]struct{})

	err = genTree(
		workdir,
		root.Command(root.EnterpriseSubcommands()),
		wroteMap,
	)
	if err != nil {
		flog.Fatalf("generating markdowns: %v", err)
	}

	// TODO delete files that aren't in the wroteMap.
}
