package gitprovider_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/externalauth/gitprovider"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestRejectedResponses(t *testing.T) {
	t.Parallel()

	pull := gitprovider.PRRef{Owner: "org", Repo: "repo", Number: 1}
	branch := gitprovider.BranchRef{Owner: "org", Repo: "repo", Branch: "feature"}
	for _, endpoint := range []struct {
		name         string
		providerType string
		message      string
		fetch        func(context.Context, gitprovider.Provider) error
	}{
		{
			name:         "GitHubJSON",
			providerType: "github",
			message:      "github request failed with status",
			fetch: func(ctx context.Context, provider gitprovider.Provider) error {
				_, err := provider.FetchPullRequestStatus(ctx, "test-token", pull)
				return err
			},
		},
		{
			name:         "GitHubDiff",
			providerType: "github",
			message:      "github diff request failed with status",
			fetch: func(ctx context.Context, provider gitprovider.Provider) error {
				_, err := provider.FetchPullRequestDiff(ctx, "test-token", pull)
				return err
			},
		},
		{
			name:         "GitLabPullRequestDiff",
			providerType: "gitlab",
			message:      "gitlab get merge request raw diffs: unexpected status",
			fetch: func(ctx context.Context, provider gitprovider.Provider) error {
				_, err := provider.FetchPullRequestDiff(ctx, "test-token", pull)
				return err
			},
		},
		{
			name:         "GitLabBranchDiff",
			providerType: "gitlab",
			message:      "gitlab compare branches: unexpected status",
			fetch: func(ctx context.Context, provider gitprovider.Provider) error {
				_, err := provider.FetchBranchDiff(ctx, "test-token", branch)
				return err
			},
		},
	} {
		t.Run(endpoint.name, func(t *testing.T) {
			t.Parallel()
			for _, response := range []struct {
				name         string
				status       int
				readError    bool
				wantRejected bool
			}{
				{
					name:         "Forbidden",
					status:       http.StatusForbidden,
					wantRejected: true,
				},
				{
					name:         "ForbiddenReadFailure",
					status:       http.StatusForbidden,
					readError:    true,
					wantRejected: true,
				},
				{
					name:         "TooManyRequests",
					status:       http.StatusTooManyRequests,
					wantRejected: true,
				},
				{
					name:         "TooManyRequestsReadFailure",
					status:       http.StatusTooManyRequests,
					readError:    true,
					wantRejected: true,
				},
				{
					name:   "OtherError",
					status: http.StatusInternalServerError,
				},
			} {
				t.Run(response.name, func(t *testing.T) {
					t.Parallel()
					clock := quartz.NewMock(t)

					// GitLab needs a project lookup before
					// comparing branches. This transport
					// answers it and sends both providers'
					// headers with requests remaining.
					transport := testutil.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
						if strings.HasSuffix(req.URL.Path, "/projects/org/repo") {
							return &http.Response{
								StatusCode: http.StatusOK,
								Header:     http.Header{"Content-Type": {"application/json"}},
								Body:       io.NopCloser(strings.NewReader(`{"default_branch":"main"}`)),
								Request:    req,
							}, nil
						}

						var body io.Reader = strings.NewReader("permission denied")
						if response.readError {
							body = iotest.ErrReader(xerrors.New("read failed"))
						}

						return &http.Response{
							StatusCode: response.status,
							Header: http.Header{
								"X-Ratelimit-Remaining": {"4990"},
								"X-Ratelimit-Reset":     {fmt.Sprint(clock.Now().Add(time.Hour).Unix())},
								"Ratelimit-Remaining":   {"4990"},
								"Ratelimit-Reset":       {fmt.Sprint(clock.Now().Add(time.Hour).Unix())},
							},
							Body:    io.NopCloser(body),
							Request: req,
						}, nil
					})
					client := &http.Client{Transport: transport}
					provider, err := gitprovider.New(endpoint.providerType, "https://provider.example", client, gitprovider.WithClock(clock))
					require.NoError(t, err)

					wantMessage := fmt.Sprintf("%s %d", endpoint.message, response.status)
					if !response.readError {
						wantMessage += ": permission denied"
					}

					err = endpoint.fetch(testutil.Context(t, testutil.WaitShort), provider)

					require.Error(t, err)
					_, rateLimited := errors.AsType[*gitprovider.RateLimitError](err)
					require.False(t, rateLimited)
					rejected, isRejected := errors.AsType[gitprovider.RejectedError](err)
					require.Equal(t, response.wantRejected, isRejected)
					if isRejected {
						require.Equal(t, wantMessage, rejected.Error())
					} else {
						require.ErrorContains(t, err, wantMessage)
					}
				})
			}
		})
	}
}
