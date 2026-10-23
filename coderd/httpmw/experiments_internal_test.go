package httpmw

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

type fakeUserExperiments map[uuid.UUID]bool

func (f fakeUserExperiments) Enabled(_ context.Context, userID uuid.UUID, ex codersdk.Experiment) bool {
	return ex == codersdk.ExperimentWorkspaceFiles && f[userID]
}

func TestRequireUserExperiment(t *testing.T) {
	t.Parallel()

	enabledUser, disabledUser := uuid.New(), uuid.New()
	experiments := fakeUserExperiments{enabledUser: true}

	for _, tc := range []struct {
		name      string
		userID    uuid.UUID
		devBypass bool
		want      int
	}{
		{name: "EnabledForUser", userID: enabledUser, want: http.StatusOK},
		{name: "DisabledForUser", userID: disabledUser, want: http.StatusForbidden},
		{name: "DevBypass", userID: disabledUser, devBypass: true, want: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler := requireUserExperiment(experiments, codersdk.ExperimentWorkspaceFiles, tc.devBypass)(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				}),
			)
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r = r.WithContext(context.WithValue(r.Context(), apiKeyContextKey{}, database.APIKey{UserID: tc.userID}))
			rw := httptest.NewRecorder()
			handler.ServeHTTP(rw, r)
			require.Equal(t, tc.want, rw.Code)
		})
	}
}
