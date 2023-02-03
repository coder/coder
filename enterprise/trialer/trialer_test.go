package trialer_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database/dbfake"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/enterprise/trialer"
)

func TestTrialer(t *testing.T) {
	t.Parallel()
	license := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
		Trial: true,
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(license))
	}))
	defer srv.Close()
	db := dbfake.New()

	gen := trialer.New(db, srv.URL, coderdenttest.Keys)
	err := gen(context.Background(), "kyle@coder.com")
	require.NoError(t, err)
	licenses, err := db.GetLicenses(context.Background())
	require.NoError(t, err)
	require.Len(t, licenses, 1)
}
