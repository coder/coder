package main

import (
	"encoding/json"
	"os"

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
	err := json.NewDecoder(os.Stdin).Decode(&c)
	if err != nil {
		flog.Fatal("decode stdin: %+v", err)
	}
}
