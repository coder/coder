package coderd_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/coder/coder/coderd"
	"github.com/go-chi/chi/v5"

	"go.uber.org/goleak"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/coderd/coderdtest"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestBuildInfo(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	buildInfo, err := client.BuildInfo(context.Background())
	require.NoError(t, err)
	require.Equal(t, buildinfo.ExternalURL(), buildInfo.ExternalURL, "external URL")
	require.Equal(t, buildinfo.Version(), buildInfo.Version, "version")
}

func TestWalk(t *testing.T) {
	r, _ := coderd.New(&coderd.Options{})
	chiRouter := r.(chi.Router)
	chi.Walk(chiRouter, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		fmt.Println(method, route)
		return nil
	})
}
