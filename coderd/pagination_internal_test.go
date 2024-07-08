package coderd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestPagination(t *testing.T) {
	t.Parallel()
	const invalidValues = "Query parameters have invalid values"
	testCases := []struct {
		Name string

		AfterID string
		Limit   string
		Offset  string

		ExpectedError  string
		ExpectedParams codersdk.Pagination
	}{
		{
			Name:          "BadAfterID",
			AfterID:       "bogus",
			ExpectedError: invalidValues,
		},
		{
			Name:          "ShortAfterID",
			AfterID:       "ff22a7b-bb6f-43d8-83e1-eefe0a1f5197",
			ExpectedError: invalidValues,
		},
		{
			Name:          "LongAfterID",
			AfterID:       "cff22a7b-bb6f-43d8-83e1-eefe0a1f51972",
			ExpectedError: invalidValues,
		},
		{
			Name:          "BadLimit",
			Limit:         "bogus",
			ExpectedError: invalidValues,
		},
		{
			Name:          "TooHighLimit",
			Limit:         "2147483648",
			ExpectedError: invalidValues,
		},
		{
			Name:          "NegativeLimit",
			Limit:         "-1",
			ExpectedError: invalidValues,
		},
		{
			Name:          "BadOffset",
			Offset:        "bogus",
			ExpectedError: invalidValues,
		},
		{
			Name:          "TooHighOffset",
			Offset:        "2147483648",
			ExpectedError: invalidValues,
		},
		{
			Name:          "NegativeOffset",
			Offset:        "-1",
			ExpectedError: invalidValues,
		},

		// Valid values
		{
			Name:    "ValidAllParams",
			AfterID: "d6c1c331-bfc8-44ef-a0d2-d2294be6195a",
			Offset:  "100",
			Limit:   "50",
			ExpectedParams: codersdk.Pagination{
				AfterID: uuid.MustParse("d6c1c331-bfc8-44ef-a0d2-d2294be6195a"),
				Limit:   50,
				Offset:  100,
			},
		},
		{
			Name:  "ValidLimit",
			Limit: "50",
			ExpectedParams: codersdk.Pagination{
				AfterID: uuid.Nil,
				Limit:   50,
			},
		},
		{
			Name:   "ValidOffset",
			Offset: "150",
			ExpectedParams: codersdk.Pagination{
				AfterID: uuid.Nil,
				Offset:  150,
			},
		},
		{
			Name:    "ValidAfterID",
			AfterID: "5f2005fc-acc4-4e5e-a7fa-be017359c60b",
			ExpectedParams: codersdk.Pagination{
				AfterID: uuid.MustParse("5f2005fc-acc4-4e5e-a7fa-be017359c60b"),
			},
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			rw := httptest.NewRecorder()
			r, err := http.NewRequestWithContext(context.Background(), "GET", "https://example.com", nil)
			require.NoError(t, err, "new request")

			// Set query params
			query := r.URL.Query()
			query.Set("after_id", c.AfterID)
			query.Set("limit", c.Limit)
			query.Set("offset", c.Offset)
			r.URL.RawQuery = query.Encode()

			params, ok := parsePagination(rw, r)
			if c.ExpectedError == "" {
				require.True(t, ok, "expect ok")
				require.Equal(t, c.ExpectedParams, params, "expected params")
			} else {
				require.False(t, ok, "expect !ok")
				require.Equal(t, http.StatusBadRequest, rw.Code, "bad request status code")
				var apiError codersdk.Error
				err := json.NewDecoder(rw.Body).Decode(&apiError)
				require.NoError(t, err, "decode response")
				require.Contains(t, apiError.Message, c.ExpectedError, "expected error")
			}
		})
	}
}
