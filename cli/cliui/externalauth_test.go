package cliui_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/coder/v2/testutil/expecter"
	"github.com/coder/serpent"
)

func TestExternalAuth(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	cmd := &serpent.Command{
		Handler: func(inv *serpent.Invocation) error {
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
	stdout := expecter.NewAttachedToInvocation(t, inv)

	done := make(chan struct{})
	go func() {
		defer close(done)
		err := inv.Run()
		assert.NoError(t, err)
	}()
	stdout.ExpectMatch(ctx, "You must authenticate with")
	stdout.ExpectMatch(ctx, "https://example.com/gitauth/github")
	stdout.ExpectMatch(ctx, "Successfully authenticated with GitHub")
	<-done
}
