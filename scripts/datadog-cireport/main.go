package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// The DataDog "cireport" API is not publicly documented,
// but implementation is available in their open-source CLI
// built for CI: https://github.com/DataDog/datadog-ci
//
// It's built using node, and took ~3 minutes to install and
// run on our Windows runner, and ~1 minute on all others.
//
// This script models that code as much as possible.
func main() {
	apiKey := os.Getenv("DATADOG_API_KEY")
	if apiKey == "" {
		log.Fatal("DATADOG_API_KEY must be set!")
	}
	if len(os.Args) <= 1 {
		log.Fatal("You must supply a filename to upload!")
	}

	// Code (almost) verbatim translated from:
	// https://github.com/DataDog/datadog-ci/blob/78d0da28e1c1af44333deabf1c9486e2ad66b8af/src/helpers/ci.ts#L194-L229
	var (
		githubServerURL  = os.Getenv("GITHUB_SERVER_URL")
		githubRepository = os.Getenv("GITHUB_REPOSITORY")
		githubSHA        = os.Getenv("GITHUB_SHA")
		githubRunID      = os.Getenv("GITHUB_RUN_ID")
		pipelineURL      = fmt.Sprintf("%s/%s/actions/runs/%s", githubServerURL, githubRepository, githubRunID)
		jobURL           = fmt.Sprintf("%s/%s/commit/%s/checks", githubServerURL, githubRepository, githubSHA)
	)
	if os.Getenv("GITHUB_RUN_ATTEMPT") != "" {
		pipelineURL += fmt.Sprintf("/attempts/%s", os.Getenv("GITHUB_RUN_ATTEMPT"))
	}

	commitMessage, err := exec.Command("git", "log", "-1", "--pretty=format:%s").CombinedOutput()
	if err != nil {
		log.Fatalf("Get commit message: %s", err)
	}
	commitData, err := exec.Command("git", "show", "-s", "--format=%an,%ae,%ad,%cn,%ce,%cd").CombinedOutput()
	if err != nil {
		log.Fatalf("Get commit data: %s", err)
	}
	commitParts := strings.Split(string(commitData), ",")

	// On pull requests, this will be set!
	branch := os.Getenv("GITHUB_HEAD_REF")
	if branch == "" {
		githubRef := os.Getenv("GITHUB_REF")
		for _, prefix := range []string{"refs/heads/", "refs/tags/"} {
			if !strings.HasPrefix(githubRef, prefix) {
				continue
			}
			branch = strings.TrimPrefix(githubRef, prefix)
		}
	}

	tags := map[string]string{
		"service":              "coder",
		"_dd.cireport_version": "2",

		"test.traits": fmt.Sprintf(`{"database":[%q], "category":[%q]}`,
			os.Getenv("DD_DATABASE"), os.Getenv("DD_CATEGORY")),

		// Additional tags found in DataDog docs. See:
		// https://docs.datadoghq.com/continuous_integration/setup_tests/junit_upload/#collecting-environment-configuration-metadata
		"os.platform":     runtime.GOOS,
		"os.architecture": runtime.GOARCH,

		"ci.job.url":         jobURL,
		"ci.pipeline.id":     githubRunID,
		"ci.pipeline.name":   os.Getenv("GITHUB_WORKFLOW"),
		"ci.pipeline.number": os.Getenv("GITHUB_RUN_NUMBER"),
		"ci.pipeline.url":    pipelineURL,
		"ci.provider.name":   "github",
		"ci.workspace_path":  os.Getenv("GITHUB_WORKSPACE"),

		"git.branch":         branch,
		"git.commit.sha":     githubSHA,
		"git.repository_url": fmt.Sprintf("%s/%s.git", githubServerURL, githubRepository),

		"git.commit.message":         string(commitMessage),
		"git.commit.author.name":     commitParts[0],
		"git.commit.author.email":    commitParts[1],
		"git.commit.author.date":     commitParts[2],
		"git.commit.committer.name":  commitParts[3],
		"git.commit.committer.email": commitParts[4],
		"git.commit.committer.date":  commitParts[5],
	}

	xmlFilePath := filepath.Clean(os.Args[1])
	xmlFileData, err := os.ReadFile(xmlFilePath)
	if err != nil {
		log.Fatalf("Read %q: %s", xmlFilePath, err)
	}
	// https://github.com/DataDog/datadog-ci/blob/78d0da28e1c1af44333deabf1c9486e2ad66b8af/src/commands/junit/api.ts#L53
	var xmlCompressedBuffer bytes.Buffer
	xmlGzipWriter := gzip.NewWriter(&xmlCompressedBuffer)
	_, err = xmlGzipWriter.Write(xmlFileData)
	if err != nil {
		log.Fatalf("Write xml: %s", err)
	}
	err = xmlGzipWriter.Close()
	if err != nil {
		log.Fatalf("Close xml gzip writer: %s", err)
	}

	// Represents FormData. See:
	// https://github.com/DataDog/datadog-ci/blob/78d0da28e1c1af44333deabf1c9486e2ad66b8af/src/commands/junit/api.ts#L27
	var multipartBuffer bytes.Buffer
	multipartWriter := multipart.NewWriter(&multipartBuffer)

	// Adds the event data. See:
	// https://github.com/DataDog/datadog-ci/blob/78d0da28e1c1af44333deabf1c9486e2ad66b8af/src/commands/junit/api.ts#L42
	eventMimeHeader := make(textproto.MIMEHeader)
	eventMimeHeader.Set("Content-Disposition", `form-data; name="event"; filename="event.json"`)
	eventMimeHeader.Set("Content-Type", "application/json")
	eventMultipartWriter, err := multipartWriter.CreatePart(eventMimeHeader)
	if err != nil {
		log.Fatalf("Create event multipart: %s", err)
	}
	eventJSON, err := json.Marshal(tags)
	if err != nil {
		log.Fatalf("Marshal tags: %s", err)
	}
	_, err = eventMultipartWriter.Write(eventJSON)
	if err != nil {
		log.Fatalf("Write event JSON: %s", err)
	}

	// This seems really strange, but better to follow the implementation. See:
	// https://github.com/DataDog/datadog-ci/blob/78d0da28e1c1af44333deabf1c9486e2ad66b8af/src/commands/junit/api.ts#L44-L55
	xmlFilename := fmt.Sprintf("%s-coder-%s-%s-%s", filepath.Base(xmlFilePath), githubSHA, pipelineURL, jobURL)
	xmlFilename = regexp.MustCompile("[^a-z0-9]").ReplaceAllString(xmlFilename, "_")

	xmlMimeHeader := make(textproto.MIMEHeader)
	xmlMimeHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="junit_xml_report_file"; filename="%s.xml.gz"`, xmlFilename))
	xmlMimeHeader.Set("Content-Type", "application/octet-stream")
	inputWriter, err := multipartWriter.CreatePart(xmlMimeHeader)
	if err != nil {
		log.Fatalf("Create xml.gz multipart: %s", err)
	}
	_, err = inputWriter.Write(xmlCompressedBuffer.Bytes())
	if err != nil {
		log.Fatalf("Write xml.gz: %s", err)
	}
	err = multipartWriter.Close()
	if err != nil {
		log.Fatalf("Close: %s", err)
	}

	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, "POST", "https://cireport-intake.datadoghq.com/api/v2/cireport", &multipartBuffer)
	if err != nil {
		log.Fatalf("Create request: %s", err)
	}
	req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	req.Header.Set("DD-API-KEY", apiKey)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("Do request: %s", err)
	}
	defer res.Body.Close()
	var msg json.RawMessage
	err = json.NewDecoder(res.Body).Decode(&msg)
	if err != nil {
		log.Fatalf("Decode response: %s", err)
	}
	msg, err = json.MarshalIndent(msg, "", "\t")
	if err != nil {
		log.Fatalf("Pretty print: %s", err)
	}
	_, _ = fmt.Println(string(msg))
	msg, err = json.MarshalIndent(tags, "", "\t")
	if err != nil {
		log.Fatalf("Marshal tags: %s", err)
	}
	_, _ = fmt.Println(string(msg))
	_, _ = fmt.Printf("Status: %d\n", res.StatusCode)
}
