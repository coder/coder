package cliui_test

import (
	"context"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/testutil"
)

func TestGitAuth(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	ptty := ptytest.New(t)
	cmd := &clibase.Cmd{
		Handler: func(inv *clibase.Invocation) error {
			var fetched atomic.Bool
			return cliui.GitAuth(inv.Context(), inv.Stdout, cliui.GitAuthOptions{
				Fetch: func(ctx context.Context) ([]codersdk.TemplateVersionGitAuth, error) {
					defer fetched.Store(true)
					return []codersdk.TemplateVersionGitAuth{{
						ID:              "github",
						Type:            codersdk.GitProviderGitHub,
						Authenticated:   fetched.Load(),
						AuthenticateURL: "https://example.com/gitauth/github?redirect=" + url.QueryEscape("/gitauth?notify"),
					}}, nil
				},
				FetchInterval: time.Millisecond,
			})
		},
	}

	inv := cmd.Invoke().WithContext(ctx)

	ptty.Attach(inv)
	done := make(chan struct{})
	go func() {
		defer close(done)
		err := inv.Run()
		assert.NoError(t, err)
	}()
	ptty.ExpectMatchContext(ctx, "You must authenticate with")
	ptty.ExpectMatchContext(ctx, "https://example.com/gitauth/github")
	ptty.ExpectMatchContext(ctx, "Successfully authenticated with GitHub")
	<-done
}
