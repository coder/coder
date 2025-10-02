package searchquery_test

import (
	"context"
	"testing"

	"strings"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/searchquery"
	"github.com/coder/coder/v2/codersdk"
)

func TestProvisionerJobs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, _ := dbtestutil.NewDB(t)
	page := codersdk.Pagination{Limit: 10}

	// Create test data
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})

	testCases := []struct {
		name           string
		query          string
		expectedErrors []string // Expected error messages (empty slice means no errors expected)
		validateFilter func(t *testing.T, filter database.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisionerParams)
	}{
		{
			name:           "EmptyQuery",
			query:          "",
			expectedErrors: nil, // No errors expected
			validateFilter: func(t *testing.T, filter database.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisionerParams) {
				if filter.OrganizationID != uuid.Nil {
					t.Errorf("Expected empty organization ID, got: %v", filter.OrganizationID)
				}
			},
		},
		{
			name:           "SingleStatus",
			query:          "status:running",
			expectedErrors: nil, // No errors expected
			validateFilter: func(t *testing.T, filter database.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisionerParams) {
				if len(filter.Status) != 1 {
					t.Errorf("Expected 1 status, got: %d", len(filter.Status))
				}
				if filter.Status[0] != database.ProvisionerJobStatusRunning {
					t.Errorf("Expected status 'running', got: %v", filter.Status[0])
				}
			},
		},
		{
			name:           "MultipleStatuses",
			query:          "status:running status:pending",
			expectedErrors: nil, // No errors expected
			validateFilter: func(t *testing.T, filter database.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisionerParams) {
				if len(filter.Status) != 2 {
					t.Errorf("Expected 2 statuses, got: %d", len(filter.Status))
				}
			},
		},
		{
			name:           "InitiatorFilter",
			query:          "initiator:" + user.Username,
			expectedErrors: nil, // No errors expected
			validateFilter: func(t *testing.T, filter database.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisionerParams) {
				if filter.InitiatorID != user.ID {
					t.Errorf("Expected initiator ID %v, got: %v", user.ID, filter.InitiatorID)
				}
			},
		},
		{
			name:           "ComplexQuery",
			query:          "status:running status:pending initiator:" + user.Username + " organization:" + org.Name,
			expectedErrors: nil, // No errors expected
			validateFilter: func(t *testing.T, filter database.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisionerParams) {
				if len(filter.Status) != 2 {
					t.Errorf("Expected 2 statuses, got: %d", len(filter.Status))
				}
				if filter.OrganizationID != org.ID {
					t.Errorf("Expected organization ID %v, got: %v", org.ID, filter.OrganizationID)
				}
				if filter.InitiatorID != user.ID {
					t.Errorf("Expected initiator ID %v, got: %v", user.ID, filter.InitiatorID)
				}
			},
		},
		{
			name:           "InvalidQuery",
			query:          "invalid:format:with:too:many:colons",
			expectedErrors: []string{"can only contain 1 ':'"},
			validateFilter: func(t *testing.T, filter database.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisionerParams) {
				// No validation needed for invalid queries
			},
		},
		{
			name:           "InvalidUser",
			query:          "initiator:nonexistentuser",
			expectedErrors: []string{"either does not exist, or you are unauthorized to view them"},
			validateFilter: func(t *testing.T, filter database.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisionerParams) {
				// No validation needed for invalid queries
			},
		},
		{
			name:           "InvalidOrganization",
			query:          "organization:nonexistentorg",
			expectedErrors: []string{"either does not exist, or you are unauthorized to view it"},
			validateFilter: func(t *testing.T, filter database.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisionerParams) {
				// No validation needed for invalid queries
			},
		},
		{
			name:           "FreeFormSearchNotSupported",
			query:          "some random search term",
			expectedErrors: []string{"Free-form search terms are not supported for provisioner jobs"},
			validateFilter: func(t *testing.T, filter database.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisionerParams) {
				// No validation needed for invalid queries
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			filter, errors := searchquery.ProvisionerJobs(ctx, db, tc.query, page)

			if len(tc.expectedErrors) == 0 {
				// No errors expected
				if len(errors) > 0 {
					t.Fatalf("Expected no errors for query %q, got: %v", tc.query, errors)
				}
				tc.validateFilter(t, filter)
			} else {
				// Specific errors expected
				if len(errors) == 0 {
					t.Fatalf("Expected errors for query %q, but got none", tc.query)
				}

				// Check that we got the expected number of errors
				if len(errors) != len(tc.expectedErrors) {
					t.Errorf("Expected %d errors, got %d: %v", len(tc.expectedErrors), len(errors), errors)
				}

				// Check that each expected error message is contained in the actual errors
				errorMessages := make([]string, len(errors))
				for i, err := range errors {
					errorMessages[i] = err.Detail
				}

				for i, expectedError := range tc.expectedErrors {
					if i >= len(errorMessages) {
						t.Errorf("Expected error %d: %q, but got no error", i, expectedError)
						continue
					}
					if !strings.Contains(errorMessages[i], expectedError) {
						t.Errorf("Expected error %d to contain %q, got: %q", i, expectedError, errorMessages[i])
					}
				}
			}
		})
	}
}
