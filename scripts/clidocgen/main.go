package main

import (
	"encoding/json"
	"log"
	"os"
	"path"
	"strings"

	"github.com/coder/coder/cli"
)

type route struct {
	Title       string  `json:"title,omitempty"`
	Description string  `json:"description,omitempty"`
	Path        string  `json:"path,omitempty"`
	IconPath    string  `json:"icon_path,omitempty"`
	State       string  `json:"state,omitempty"`
	Children    []route `json:"children,omitempty"`
}

type manifest struct {
	Versions []string `json:"versions,omitempty"`
	Routes   []route  `json:"routes,omitempty"`
}

func main() {
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "CODER_") {
			split := strings.SplitN(env, "=", 2)
			if err := os.Unsetenv(split[0]); err != nil {
				log.Fatal("Unable to unset ", split[0], ": ", err)
			}
		}
	}
	for k, v := range map[string]string{
		"CODER_CONFIG_DIR":      "~/.config/coderv2",
		"CODER_CACHE_DIRECTORY": "~/.cache/coder",
	} {
		if err := os.Setenv(k, v); err != nil {
			log.Fatal("Unable to set default value for ", k, ": ", err)
		}
	}

	// Get the cmd CLI
	cmd := cli.Root(cli.AGPL())

	// Get paths
	basePath := os.Getenv("BASE_PATH")
	if basePath == "" {
		log.Fatal("BASE_PATH should be defined")
	}
	markdownDocsDir := path.Join(basePath, "docs/cli")
	manifestFilepath := path.Join(basePath, "docs/manifest.json")

	// Generate markdown
	err := generateDocsTree(cmd, markdownDocsDir)
	if err != nil {
		log.Fatal("Error on generating CLI markdown docs: ", err)
	}

	// Create CLI routes
	var cliRoutes []route
	files, err := os.ReadDir(markdownDocsDir)
	if err != nil {
		log.Fatal("Error on loading docs/cli files: ", err)
	}
	for _, file := range files {
		// Remove file extension and prefix
		title := strings.Replace(file.Name(), ".md", "", 1)
		title = strings.Replace(title, "coder_", "", 1)
		title = strings.Replace(title, "_", " ", -1)

		cliRoutes = append(cliRoutes, route{
			Title: title,
			Path:  "./cli/" + file.Name(),
		})
	}

	// Read manifest
	jsonFile, err := os.ReadFile(manifestFilepath)
	if err != nil {
		log.Fatal("Error on open docs/manifest.json: ", err)
	}
	var manifest manifest
	err = json.Unmarshal(jsonFile, &manifest)
	if err != nil {
		log.Fatal("Error on unmarshal manifest.json: ", err)
	}

	// Update manifest
	for i, r := range manifest.Routes {
		if r.Title != "Command Line" {
			continue
		}

		manifest.Routes[i].Children = cliRoutes
		break
	}
	manifestFile, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		log.Fatal("Error on marshal manifest.json: ", err)
	}
	err = os.WriteFile(manifestFilepath, manifestFile, 0o644) // #nosec
	if err != nil {
		log.Fatal("Error on write update on manifest.json: ", err)
	}
}
