// Package githubrunners reconciles GitHub scale sets with disposable workspaces.
package githubrunners

import (
	"context"
	"time"

	"github.com/actions/scaleset"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/database"
)

// Session is the acknowledged message stream for a scale set.
type Session interface {
	GetMessage(context.Context, int, int) (*scaleset.RunnerScaleSetMessage, error)
	DeleteMessage(context.Context, int) error
	AcquireJobs(context.Context, []int64) ([]int64, error)
	Session() scaleset.RunnerScaleSetSession
	Close(context.Context) error
}

// Client is the subset of GitHub's API used by the pool controller.
type Client interface {
	GetRunnerScaleSet(context.Context, int, string) (*scaleset.RunnerScaleSet, error)
	CreateRunnerScaleSet(context.Context, *scaleset.RunnerScaleSet) (*scaleset.RunnerScaleSet, error)
	DeleteRunnerScaleSet(context.Context, int) error
	GenerateJitRunnerConfig(context.Context, *scaleset.RunnerScaleSetJitRunnerSetting, int) (*scaleset.RunnerScaleSetJitRunnerConfig, error)
	GetRunnerByName(context.Context, string) (*scaleset.RunnerReference, error)
	RemoveRunner(context.Context, int64) error
	OpenSession(context.Context, int, string) (Session, error)
}

type client struct{ *scaleset.Client }

// NewClient creates a GitHub.com scale-set client with bounded requests.
func NewClient(pool database.GithubRunnerPool) (Client, error) {
	c, err := scaleset.NewClientWithPersonalAccessToken(scaleset.NewClientWithPersonalAccessTokenConfig{
		GitHubConfigURL: pool.GithubConfigUrl, PersonalAccessToken: pool.GithubToken,
		SystemInfo: scaleset.SystemInfo{System: "coder", Version: buildinfo.Version(), Subsystem: "controller"},
	}, scaleset.WithTimeout(70*time.Second), scaleset.WithRetryMax(2), scaleset.WithLogger(nil))
	if err != nil {
		return nil, err
	}
	return client{c}, nil
}

func (c client) OpenSession(ctx context.Context, id int, owner string) (Session, error) {
	return c.MessageSessionClient(ctx, id, owner)
}

// ScaleSetName includes the pool ID so registration can recover after a crash.
func ScaleSetName(pool database.GithubRunnerPool) string {
	return pool.Name + "-" + pool.ID.String()[:8]
}
