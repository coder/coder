package cliui_test

import (
	"context"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

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
	cmd := &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			var fetched atomic.Bool
			return cliui.GitAuth(cmd.Context(), cmd.OutOrStdout(), cliui.GitAuthOptions{
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
	cmd.SetOutput(ptty.Output())
	cmd.SetIn(ptty.Input())
	done := make(chan struct{})
	go func() {
		defer close(done)
		err := cmd.Execute()
		assert.NoError(t, err)
	}()
	ptty.ExpectMatchContext(ctx, "You must authenticate with")
	ptty.ExpectMatchContext(ctx, "https://example.com/gitauth/github")
	ptty.ExpectMatchContext(ctx, "Successfully authenticated with GitHub")
	<-done
}
