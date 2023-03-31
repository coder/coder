package coderd_test

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/healthcheck"
	"github.com/coder/coder/testutil"
)

func TestDebug(t *testing.T) {
	t.Parallel()
	t.Run("Health/OK", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithTimeout(context.Background(), testutil.WaitShort)
			client      = coderdtest.New(t, &coderdtest.Options{
				HealthcheckFunc: func(context.Context) (*healthcheck.Report, error) {
					return &healthcheck.Report{}, nil
				},
			})
			_ = coderdtest.CreateFirstUser(t, client)
		)
		defer cancel()

		res, err := client.Request(ctx, "GET", "/debug/health", nil)
		require.NoError(t, err)
		defer res.Body.Close()
		_, _ = io.ReadAll(res.Body)
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("Health/Timeout", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithTimeout(context.Background(), testutil.WaitShort)
			client      = coderdtest.New(t, &coderdtest.Options{
				HealthcheckTimeout: time.Microsecond,
				HealthcheckFunc: func(context.Context) (*healthcheck.Report, error) {
					t := time.NewTimer(time.Second)
					defer t.Stop()

					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-t.C:
						return &healthcheck.Report{}, nil
					}
				},
			})
			_ = coderdtest.CreateFirstUser(t, client)
		)
		defer cancel()

		res, err := client.Request(ctx, "GET", "/api/v2/debug/health", nil)
		require.NoError(t, err)
		defer res.Body.Close()
		_, _ = io.ReadAll(res.Body)
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})
}
