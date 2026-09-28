package codersdk_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestCreateUserRequestJSON(t *testing.T) {
	t.Parallel()

	marshalTest := func(t *testing.T, req codersdk.CreateUserRequest) {
		t.Helper()
		data, err := json.Marshal(req)
		require.NoError(t, err)
		var req2 codersdk.CreateUserRequest
		err = json.Unmarshal(data, &req2)
		require.NoError(t, err)
		require.Equal(t, req, req2)
	}

	t.Run("MultipleOrganizations", func(t *testing.T) {
		t.Parallel()

		req := codersdk.CreateUserRequest{
			Email:           "alice@coder.com",
			Username:        "alice",
			Name:            "Alice User",
			Password:        "",
			UserLoginType:   codersdk.LoginTypePassword,
			OrganizationIDs: []uuid.UUID{uuid.New(), uuid.New()},
		}
		marshalTest(t, req)
	})

	t.Run("SingleOrganization", func(t *testing.T) {
		t.Parallel()

		req := codersdk.CreateUserRequest{
			Email:           "alice@coder.com",
			Username:        "alice",
			Name:            "Alice User",
			Password:        "",
			UserLoginType:   codersdk.LoginTypePassword,
			OrganizationIDs: []uuid.UUID{uuid.New()},
		}
		marshalTest(t, req)
	})

	t.Run("NoOrganization", func(t *testing.T) {
		t.Parallel()

		req := codersdk.CreateUserRequest{
			Email:           "alice@coder.com",
			Username:        "alice",
			Name:            "Alice User",
			Password:        "",
			UserLoginType:   codersdk.LoginTypePassword,
			OrganizationIDs: []uuid.UUID{},
		}
		marshalTest(t, req)
	})
}
