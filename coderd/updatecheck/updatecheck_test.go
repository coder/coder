package updatecheck_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/go-github/v43/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/updatecheck"
	"github.com/coder/coder/v2/testutil"
)

func TestChecker_Notify(t *testing.T) {
	t.Parallel()

	responses := []github.RepositoryRelease{
		{TagName: github.String("v1.2.3"), HTMLURL: github.String("https://someurl.com")},
		{TagName: github.String("v1.2.4"), HTMLURL: github.String("https://someurl.com")},
		{TagName: github.String("v1.2.4"), HTMLURL: github.String("https://someurl.com")},
		{TagName: github.String("v1.2.5"), HTMLURL: github.String("https://someurl.com")},
	}
	responseC := make(chan github.RepositoryRelease, len(responses))
	for _, r := range responses {
		responseC <- r
	}

	wantVersion := []string{"v1.2.3", "v1.2.4", "v1.2.5"}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case resp := <-responseC:
			b, err := json.Marshal(resp)
			assert.NoError(t, err)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(b)
		}
	}))
	defer srv.Close()

	db := dbmem.New()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Named(t.Name())
	notify := make(chan updatecheck.Result, len(wantVersion))
	c := updatecheck.New(db, logger, updatecheck.Options{
		Interval: 1 * time.Nanosecond, // Zero means unset.
		URL:      srv.URL,
		Notify: func(r updatecheck.Result) {
			select {
			case notify <- r:
			default:
				t.Error("unexpected notification")
			}
		},
	})
	defer c.Close()

	ctx := testutil.Context(t, testutil.WaitLong)

	for i := 0; i < len(wantVersion); i++ {
		select {
		case <-ctx.Done():
			t.Error("timed out waiting for notification")
		case r := <-notify:
			assert.Equal(t, wantVersion[i], r.Version)
		}
	}
}

func TestChecker_Latest(t *testing.T) {
	t.Parallel()

	rr := github.RepositoryRelease{
		TagName: github.String("v1.2.3"),
		HTMLURL: github.String("https://someurl.com"),
	}

	tests := []struct {
		name    string
		release github.RepositoryRelease
		wantR   updatecheck.Result
		wantErr bool
	}{
		{
			name:    "check latest",
			release: rr,
			wantR: updatecheck.Result{
				Version: "v1.2.3",
				URL:     "https://someurl.com",
			},
			wantErr: false,
		},
		{
			name:    "missing release data",
			release: github.RepositoryRelease{},
			wantErr: true,
		},
		{
			name:    "error",
			release: rr,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.wantErr {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				rrJSON, err := json.Marshal(rr)
				assert.NoError(t, err)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write(rrJSON)
			}))
			defer srv.Close()

			db := dbmem.New()
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Named(t.Name())
			c := updatecheck.New(db, logger, updatecheck.Options{
				URL: srv.URL,
			})
			defer c.Close()

			ctx := testutil.Context(t, testutil.WaitLong)
			_ = ctx

			gotR, err := c.Latest(ctx)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			// Zero out the time so we can compare the rest of the struct.
			gotR.Checked = time.Time{}
			require.Equal(t, tt.wantR, gotR, "wrong version")
		})
	}
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}
