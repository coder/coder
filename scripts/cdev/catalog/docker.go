package catalog

import (
	"context"

	"github.com/ory/dockertest/v3"
)

var _ Service[*dockertest.Pool] = (*Docker)(nil)

func OnDocker() string {
	return (&Docker{}).Name()
}

type Docker struct {
	pool *dockertest.Pool
}

func NewDocker() *Docker {
	return &Docker{}
}

func (d *Docker) Name() string {
	return "docker"
}

func (d *Docker) DependsOn() []string {
	return []string{}
}

func (d *Docker) Start(ctx context.Context, _ *Catalog) error {
	pool, err := dockertest.NewPool("")
	if err != nil {
		return err
	}
	d.pool = pool
	return nil
}

func (d *Docker) Stop(ctx context.Context) error {
	return nil
}

func (d *Docker) Result() *dockertest.Pool {
	return d.pool
}
