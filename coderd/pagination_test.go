package coderd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/util/ptr"
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

			params, ok := coderd.ParsePagination(rw, r)
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

func TestPaginationBounded(t *testing.T) {
	t.Parallel()
	const maxLimit = 100
	testCases := []struct {
		Name string

		// Limit is omitted from the query when nil.
		Limit  *string
		Offset string

		ExpectedError  string
		ExpectedParams codersdk.Pagination
	}{
		{
			Name:           "OmittedLimitDefaultsToMax",
			ExpectedParams: codersdk.Pagination{Limit: maxLimit},
		},
		{
			Name:           "MaxLimit",
			Limit:          ptr.Ref("100"),
			ExpectedParams: codersdk.Pagination{Limit: maxLimit},
		},
		{
			Name:           "BelowMaxLimit",
			Limit:          ptr.Ref("25"),
			Offset:         "50",
			ExpectedParams: codersdk.Pagination{Limit: 25, Offset: 50},
		},
		{
			Name:          "ZeroLimit",
			Limit:         ptr.Ref("0"),
			ExpectedError: "must be a positive integer no greater than 100",
		},
		{
			Name:          "AboveMaxLimit",
			Limit:         ptr.Ref("101"),
			ExpectedError: "must be a positive integer no greater than 100",
		},
		{
			Name:          "NegativeLimit",
			Limit:         ptr.Ref("-1"),
			ExpectedError: "must be a valid 32-bit positive integer: value is negative",
		},
		{
			Name:          "UnparseableLimit",
			Limit:         ptr.Ref("bogus"),
			ExpectedError: "must be a valid 32-bit positive integer",
		},
	}

	for _, c := range testCases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			rw := httptest.NewRecorder()
			r, err := http.NewRequestWithContext(context.Background(), "GET", "https://example.com", nil)
			require.NoError(t, err, "new request")

			query := r.URL.Query()
			if c.Limit != nil {
				query.Set("limit", *c.Limit)
			}
			if c.Offset != "" {
				query.Set("offset", c.Offset)
			}
			r.URL.RawQuery = query.Encode()

			params, ok := coderd.ParsePaginationBounded(rw, r, maxLimit)
			if c.ExpectedError == "" {
				require.True(t, ok, "expect ok")
				require.Equal(t, c.ExpectedParams, params, "expected params")
				return
			}

			require.False(t, ok, "expect !ok")
			require.Equal(t, http.StatusBadRequest, rw.Code, "bad request status code")
			var apiError codersdk.Error
			require.NoError(t, json.NewDecoder(rw.Body).Decode(&apiError), "decode response")
			require.Len(t, apiError.Validations, 1, "one validation error")
			require.Equal(t, "limit", apiError.Validations[0].Field)
			require.Contains(t, apiError.Validations[0].Detail, c.ExpectedError)
		})
	}
}
