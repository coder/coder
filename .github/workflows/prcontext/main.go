package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/coder/flog"
)

// githubContext is structured as documented here:
// https://docs.github.com/en/actions/learn-github-actions/contexts#github-context.
type githubContext struct {
	EventName string `json:"event_name"`
	Event     struct {
		PullRequest struct {
			Body string `json:"body"`
		}
	} `json:"event"`
}

func main() {
	var c githubContext
	err := json.Unmarshal([]byte(os.Getenv("GITHUB_CONTEXT")), &c)
	if err != nil {
		flog.Fatal("decode stdin: %+v", err)
	}
	flog.Info("detected event %q", c.EventName)
	if c.EventName != "pull_request" {
		flog.Info("aborting since not Pull Request")
		return
	}

	_, _ = fmt.Printf("---\n%s\n---\n", c.Event.PullRequest.Body)

	skips := parseBody(c.Event.PullRequest.Body)
	_, _ = fmt.Printf("::set-output name=skips::[%s]\n", strings.Join(skips, " "))
}
