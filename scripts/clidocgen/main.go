package main

import (
	"encoding/json"
	"log"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/spf13/cobra/doc"

	"github.com/coder/coder/buildinfo"
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
	// Set default configs for the docs
	err := os.Setenv("CODER_CONFIG_DIR", "~/.config/coderv2")
	if err != nil {
		log.Fatal("Unable to set default value for CODER_CONFIG_DIR: ", err)
	}
	err = os.Setenv("CODER_CACHE_DIRECTORY", "~/.cache/coder")
	if err != nil {
		log.Fatal("Unable to set default value for CODER_CACHE_DIRECTORY: ", err)
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
	err = doc.GenMarkdownTree(cmd, markdownDocsDir)
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

		filepath := path.Join(markdownDocsDir, file.Name())
		openFile, err := os.ReadFile(filepath)
		if err != nil {
			log.Fatal("Error on open file at ", filepath, ": ", err)
		}
		content := string(openFile)

		// Remove non printable strings from generated markdown
		// https://github.com/spf13/cobra/issues/1878
		const ansi = "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"
		ansiRegex := regexp.MustCompile(ansi)
		content = ansiRegex.ReplaceAllString(content, "")

		// Remove the version and its right space, since during this script running
		// there is no build info available
		content = strings.ReplaceAll(content, buildinfo.Version()+" ", "")

		// Remove references to the current working directory
		dir, err := os.Getwd()
		if err != nil {
			log.Fatal("Error on getting the current directory:", err)
		}
		content = strings.ReplaceAll(content, dir, "<current-directory>")

		err = os.WriteFile(filepath, []byte(content), 0644) // #nosec
		if err != nil {
			log.Fatal("Error on save file at ", filepath, ": ", err)
		}
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
	err = os.WriteFile(manifestFilepath, manifestFile, 0644) // #nosec
	if err != nil {
		log.Fatal("Error on write update on manifest.json: ", err)
	}
}
