package terraform_test

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/afero"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/provisioner/terraform"
	"github.com/coder/coder/v2/testutil"
)

const (
	cachePath = "/tmp/coder/provisioner-0/tf"
)

func TestOnePluginIsStale(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	fs := afero.NewMemMapFs()
	now := time.Now()
	logger := slogtest.Make(t, nil).Named("cleanup-test")

	terraform.CleanStaleTerraformPlugins(ctx, cachePath, fs, now, logger)

}
