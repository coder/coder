package trialer_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/trialer"
)

func TestTrialer(t *testing.T) {
	t.Parallel()
	license := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
		Trial: true,
	})
	type capture struct {
		contentType string
		body        map[string]any
	}
	captured := make(chan capture, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := capture{contentType: r.Header.Get("Content-Type")}
		if err := json.NewDecoder(r.Body).Decode(&c.body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		captured <- c
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(license))
	}))
	defer srv.Close()
	db, _ := dbtestutil.NewDB(t)
	err := db.InsertDeploymentID(context.Background(), "test-deployment")
	require.NoError(t, err)

	gen := trialer.New(db, srv.URL, coderdenttest.Keys)
	err = gen.Generate(context.Background(), codersdk.LicensorTrialRequest{
		Email:       "kyle+colin@coder.com",
		FirstName:   "Kyle",
		LastName:    "Carberry",
		PhoneNumber: "+1 555 0100",
		JobTitle:    "Platform Engineer",
		CompanyName: "Coder",
		Country:     "United States",
		Developers:  "51 - 100",
	})
	require.NoError(t, err)
	licenses, err := db.GetLicenses(context.Background())
	require.NoError(t, err)
	require.Len(t, licenses, 1)

	require.Len(t, captured, 1)
	got := <-captured
	require.Equal(t, "application/json", got.contentType)
	require.Equal(t, map[string]any{
		"deployment_id": "test-deployment",
		"source":        "Product",
		"email":         "kyle+colin@coder.com",
		"first_name":    "Kyle",
		"last_name":     "Carberry",
		"phone_number":  "+1 555 0100",
		"job_title":     "Platform Engineer",
		"company_name":  "Coder",
		"country":       "United States",
		"developers":    "51 - 100",
	}, got.body)
}

func TestTrialerRequest(t *testing.T) {
	t.Parallel()

	req := codersdk.LicensorTrialRequest{
		DeploymentID: "test-deployment",
		Email:        "coder@coder.com",
		FirstName:    "Coder",
		LastName:     "McCoder",
		PhoneNumber:  "+1 555 0100",
		JobTitle:     "Platform Engineer",
		CompanyName:  "Coder",
		Country:      "United States",
		Developers:   "51 - 100",
	}

	// Request reads neither the store nor the keys.
	requester := func(url string) *trialer.Trialer {
		return trialer.New(nil, url, nil)
	}

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		want := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{Trial: true})

		// The handler and the assertions run on different goroutines, so the
		// captured request travels over a channel to stay race-free.
		type capture struct {
			method      string
			path        string
			contentType string
			body        map[string]any
		}
		captured := make(chan capture, 1)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := capture{
				method:      r.Method,
				path:        r.URL.Path,
				contentType: r.Header.Get("Content-Type"),
			}
			if err := json.NewDecoder(r.Body).Decode(&c.body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			captured <- c
			// The licensor responds with the bare JWT. A trailing newline must not
			// end up in the stored license.
			_, _ = w.Write([]byte(want + "\n"))
		}))
		defer srv.Close()

		raw, err := requester(srv.URL).Request(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, want, raw)

		require.Len(t, captured, 1)
		got := <-captured
		require.Equal(t, http.MethodPost, got.method)
		require.Equal(t, "/", got.path)
		require.Equal(t, "application/json", got.contentType)
		require.Equal(t, map[string]any{
			"deployment_id": "test-deployment",
			"source":        "Product",
			"email":         req.Email,
			"first_name":    req.FirstName,
			"last_name":     req.LastName,
			"phone_number":  req.PhoneNumber,
			"job_title":     req.JobTitle,
			"company_name":  req.CompanyName,
			"country":       req.Country,
			"developers":    req.Developers,
		}, got.body)
	})

	t.Run("LicensorRejects", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"this deployment already had a trial"}`))
		}))
		defer srv.Close()

		_, err := requester(srv.URL).Request(context.Background(), req)
		licensorErr, ok := errors.AsType[*trialer.LicensorError](err)
		require.True(t, ok, "expected a LicensorError, got %v", err)
		require.Equal(t, "this deployment already had a trial", licensorErr.Message)
	})

	t.Run("LicensorRejectsWithoutJSON", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("<html>gateway timeout</html>"))
		}))
		defer srv.Close()

		// A body that is not the licensor's error shape must still surface as a
		// LicensorError carrying the status, not as an unmarshal failure.
		_, err := requester(srv.URL).Request(context.Background(), req)
		licensorErr, ok := errors.AsType[*trialer.LicensorError](err)
		require.True(t, ok, "expected a LicensorError, got %v", err)
		require.Equal(t, "500 Internal Server Error", licensorErr.Message)
	})

	t.Run("EmptyLicense", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		_, err := requester(srv.URL).Request(context.Background(), req)
		require.Error(t, err)
		_, ok := errors.AsType[*trialer.LicensorError](err)
		require.False(t, ok, "an empty body is a protocol failure, not a licensor rejection")
	})

	t.Run("Unreachable", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		url := srv.URL
		srv.Close()

		_, err := requester(url).Request(context.Background(), req)
		require.Error(t, err)
		_, ok := errors.AsType[*trialer.LicensorError](err)
		require.False(t, ok, "a transport failure is not a licensor rejection")
	})
}
