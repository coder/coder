package httpmw_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/justinas/nosurf"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpmw"
)

func TestCSRFExempt(t *testing.T) {
	t.Parallel()

	cases := []struct {
		Name   string
		Path   string
		Exempt bool
	}{
		{
			Name:   "Root",
			Path:   "/",
			Exempt: true,
		},
	}

	mw := httpmw.CSRF(false)
	csrfmw := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).(*nosurf.CSRFHandler)

	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			exempt := csrfmw.IsExempt(&http.Request{URL: &url.URL{Path: c.Path}})
			require.Equal(t, c.Exempt, exempt)
		})
	}
}
