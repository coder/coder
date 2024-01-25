package httpmw_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/justinas/nosurf"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

func TestCSRFExemptList(t *testing.T) {
	t.Parallel()

	cases := []struct {
		Name   string
		URL    string
		Exempt bool
	}{
		{
			Name:   "Root",
			URL:    "https://example.com",
			Exempt: true,
		},
		{
			Name:   "WorkspacePage",
			URL:    "https://coder.com/workspaces",
			Exempt: true,
		},
		{
			Name:   "SubApp",
			URL:    "https://app--dev--coder--user--apps.coder.com/",
			Exempt: true,
		},
		{
			Name:   "PathApp",
			URL:    "https://coder.com/@USER/test.instance/apps/app",
			Exempt: true,
		},
		{
			Name:   "API",
			URL:    "https://coder.com/api/v2",
			Exempt: false,
		},
		{
			Name:   "APIMe",
			URL:    "https://coder.com/api/v2/me",
			Exempt: false,
		},
	}

	mw := httpmw.CSRF(false)
	csrfmw := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).(*nosurf.CSRFHandler)

	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			r, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.URL, nil)
			require.NoError(t, err)

			r.AddCookie(&http.Cookie{Name: codersdk.SessionTokenCookie, Value: "test"})
			exempt := csrfmw.IsExempt(r)
			require.Equal(t, c.Exempt, exempt)
		})
	}
}
