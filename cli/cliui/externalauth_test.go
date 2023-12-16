package cliui_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestExternalAuth(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	ptty := ptytest.New(t)
	cmd := &clibase.Cmd{
		Handler: func(inv *clibase.Invocation) error {
			var fetched atomic.Bool
			return cliui.ExternalAuth(inv.Context(), inv.Stdout, cliui.ExternalAuthOptions{
				Fetch: func(ctx context.Context) ([]codersdk.TemplateVersionExternalAuth, error) {
					defer fetched.Store(true)
					return []codersdk.TemplateVersionExternalAuth{{
						ID:              "github",
						DisplayName:     "GitHub",
						Type:            codersdk.EnhancedExternalAuthProviderGitHub.String(),
						Authenticated:   fetched.Load(),
						AuthenticateURL: "https://example.com/gitauth/github",
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
