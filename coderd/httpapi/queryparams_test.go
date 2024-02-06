package httpapi_test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
)

type queryParamTestCase[T any] struct {
	QueryParam string
	// No set does not set the query param, rather than setting the empty value
	NoSet                 bool
	Value                 string
	Default               T
	Expected              T
	ExpectedErrorContains string
	Parse                 func(r *http.Request, def T, queryParam string) T
}

func TestParseQueryParams(t *testing.T) {
	t.Parallel()

	t.Run("Enum", func(t *testing.T) {
		t.Parallel()

		expParams := []queryParamTestCase[database.ResourceType]{
			{
				QueryParam: "resource_type",
				Value:      string(database.ResourceTypeWorkspace),
				Expected:   database.ResourceTypeWorkspace,
			},
			{
				QueryParam:            "bad_type",
				Value:                 "foo",
				ExpectedErrorContains: "not a valid value",
			},
		}

		parser := httpapi.NewQueryParamParser()
		testQueryParams(t, expParams, parser, func(vals url.Values, def database.ResourceType, queryParam string) database.ResourceType {
			return httpapi.ParseCustom(parser, vals, def, queryParam, httpapi.ParseEnum[database.ResourceType])
		})
	})

	t.Run("EnumList", func(t *testing.T) {
		t.Parallel()

		expParams := []queryParamTestCase[[]database.ResourceType]{
			{
				QueryParam: "resource_type",
				Value:      fmt.Sprintf("%s,%s", database.ResourceTypeWorkspace, database.ResourceTypeApiKey),
				Expected:   []database.ResourceType{database.ResourceTypeWorkspace, database.ResourceTypeApiKey},
			},
		}

		parser := httpapi.NewQueryParamParser()
		testQueryParams(t, expParams, parser, func(vals url.Values, def []database.ResourceType, queryParam string) []database.ResourceType {
			return httpapi.ParseCustomList(parser, vals, def, queryParam, httpapi.ParseEnum[database.ResourceType])
		})
	})

	t.Run("Time", func(t *testing.T) {
		t.Parallel()

		expParams := []queryParamTestCase[time.Time]{
			{
				QueryParam: "date",
				Value:      "2023-01-16T00:00:00+12:00",
				Expected:   time.Date(2023, 1, 15, 12, 0, 0, 0, time.UTC),
			},
			{
				QueryParam:            "bad_date",
				Value:                 "2010",
				ExpectedErrorContains: "must be a valid date format",
			},
		}

		parser := httpapi.NewQueryParamParser()
		testQueryParams(t, expParams, parser, func(vals url.Values, def time.Time, queryParam string) time.Time {
			return parser.Time3339Nano(vals, time.Time{}, queryParam)
		})
	})

	t.Run("UUID", func(t *testing.T) {
		t.Parallel()
		me := uuid.New()
		expParams := []queryParamTestCase[uuid.UUID]{
			{
				QueryParam: "valid_id",
				Value:      "afe39fbf-0f52-4a62-b0cc-58670145d773",
				Expected:   uuid.MustParse("afe39fbf-0f52-4a62-b0cc-58670145d773"),
			},
			{
				QueryParam: "me",
				Value:      "me",
				Expected:   me,
			},
			{
				QueryParam:            "invalid_id",
				Value:                 "bogus",
				ExpectedErrorContains: "invalid UUID length",
			},
			{
				QueryParam:            "long_id",
				Value:                 "afe39fbf-0f52-4a62-b0cc-58670145d773-123",
				ExpectedErrorContains: "invalid UUID length",
			},
			{
				QueryParam: "no_value",
				NoSet:      true,
				Default:    uuid.MustParse("11111111-1111-1111-1111-111111111111"),
				Expected:   uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			},
			{
				QueryParam: "empty",
				Value:      "",
				Expected:   uuid.Nil,
			},
		}

		parser := httpapi.NewQueryParamParser()
		testQueryParams(t, expParams, parser, func(vals url.Values, def uuid.UUID, queryParam string) uuid.UUID {
			return parser.UUIDorMe(vals, def, me, queryParam)
		})
	})

	t.Run("String", func(t *testing.T) {
		t.Parallel()
		expParams := []queryParamTestCase[string]{
			{
				QueryParam: "valid_string",
				Value:      "random",
				Expected:   "random",
			},
			{
				QueryParam: "empty",
				Value:      "",
				Expected:   "",
			},
			{
				QueryParam: "no_value",
				NoSet:      true,
				Default:    "default",
				Expected:   "default",
			},
		}

		parser := httpapi.NewQueryParamParser()
		testQueryParams(t, expParams, parser, parser.String)
	})

	t.Run("Boolean", func(t *testing.T) {
		t.Parallel()
		expParams := []queryParamTestCase[bool]{
			{
				QueryParam: "valid_true",
				Value:      "true",
				Expected:   true,
			},
			{
				QueryParam: "casing",
				Value:      "True",
				Expected:   true,
			},
			{
				QueryParam: "all_caps",
				Value:      "TRUE",
				Expected:   true,
			},
			{
				QueryParam: "no_value_true_def",
				NoSet:      true,
				Default:    true,
				Expected:   true,
			},
			{
				QueryParam: "no_value",
				NoSet:      true,
				Expected:   false,
			},

			{
				QueryParam:            "invalid_boolean",
				Value:                 "yes",
				Expected:              false,
				ExpectedErrorContains: "must be a valid boolean",
			},
		}

		parser := httpapi.NewQueryParamParser()
		testQueryParams(t, expParams, parser, parser.Boolean)
	})

	t.Run("Int", func(t *testing.T) {
		t.Parallel()
		expParams := []queryParamTestCase[int]{
			{
				QueryParam: "valid_integer",
				Value:      "100",
				Expected:   100,
			},
			{
				QueryParam: "empty",
				Value:      "",
				Expected:   0,
			},
			{
				QueryParam: "no_value",
				NoSet:      true,
				Default:    5,
				Expected:   5,
			},
			{
				QueryParam: "negative",
				Value:      "-10",
				Expected:   -10,
				Default:    5,
			},
			{
				QueryParam:            "invalid_integer",
				Value:                 "bogus",
				Expected:              0,
				ExpectedErrorContains: "must be a valid integer",
			},
		}

		parser := httpapi.NewQueryParamParser()
		testQueryParams(t, expParams, parser, parser.Int)
	})

	t.Run("UInt", func(t *testing.T) {
		t.Parallel()
		expParams := []queryParamTestCase[uint64]{
			{
				QueryParam: "valid_integer",
				Value:      "100",
				Expected:   100,
			},
			{
				QueryParam: "empty",
				Value:      "",
				Expected:   0,
			},
			{
				QueryParam: "no_value",
				NoSet:      true,
				Default:    5,
				Expected:   5,
			},
			{
				QueryParam:            "negative",
				Value:                 "-10",
				Default:               5,
				ExpectedErrorContains: "must be a valid positive integer",
			},
			{
				QueryParam:            "invalid_integer",
				Value:                 "bogus",
				Expected:              0,
				ExpectedErrorContains: "must be a valid positive integer",
			},
		}

		parser := httpapi.NewQueryParamParser()
		testQueryParams(t, expParams, parser, parser.UInt)
	})

	t.Run("UUIDs", func(t *testing.T) {
		t.Parallel()
		expParams := []queryParamTestCase[[]uuid.UUID]{
			{
				QueryParam: "valid_ids_with_spaces",
				Value:      "6c8ef17d-5dd8-4b92-bac9-41944f90f237, 65fb05f3-12c8-4a0a-801f-40439cf9e681 , 01b94888-1eab-4bbf-aed0-dc7a8010da97",
				Expected: []uuid.UUID{
					uuid.MustParse("6c8ef17d-5dd8-4b92-bac9-41944f90f237"),
					uuid.MustParse("65fb05f3-12c8-4a0a-801f-40439cf9e681"),
					uuid.MustParse("01b94888-1eab-4bbf-aed0-dc7a8010da97"),
				},
			},
			{
				QueryParam: "empty",
				Value:      "",
				Default:    []uuid.UUID{},
				Expected:   []uuid.UUID{},
			},
			{
				QueryParam: "no_value",
				NoSet:      true,
				Default:    []uuid.UUID{},
				Expected:   []uuid.UUID{},
			},
			{
				QueryParam: "default",
				NoSet:      true,
				Default:    []uuid.UUID{uuid.Nil},
				Expected:   []uuid.UUID{uuid.Nil},
			},
			{
				QueryParam:            "invalid_id_in_set",
				Value:                 "6c8ef17d-5dd8-4b92-bac9-41944f90f237,bogus",
				Expected:              []uuid.UUID{},
				Default:               []uuid.UUID{},
				ExpectedErrorContains: "bogus",
			},
		}

		parser := httpapi.NewQueryParamParser()
		testQueryParams(t, expParams, parser, parser.UUIDs)
	})

	t.Run("Required", func(t *testing.T) {
		t.Parallel()

		parser := httpapi.NewQueryParamParser()
		parser.RequiredNotEmpty("test_value")
		parser.UUID(url.Values{}, uuid.New(), "test_value")
		require.Len(t, parser.Errors, 1)

		parser = httpapi.NewQueryParamParser()
		parser.RequiredNotEmpty("test_value")
		parser.String(url.Values{"test_value": {""}}, "", "test_value")
		require.Len(t, parser.Errors, 1)
	})
}

func testQueryParams[T any](t *testing.T, testCases []queryParamTestCase[T], parser *httpapi.QueryParamParser, parse func(vals url.Values, def T, queryParam string) T) {
	v := url.Values{}
	for _, c := range testCases {
		if c.NoSet {
			continue
		}
		v.Set(c.QueryParam, c.Value)
	}

	for _, c := range testCases {
		// !! Do not run these in parallel !!
		t.Run(c.QueryParam, func(t *testing.T) {
			value := parse(v, c.Default, c.QueryParam)
			require.Equal(t, c.Expected, value, fmt.Sprintf("param=%q value=%q", c.QueryParam, c.Value))
			if c.ExpectedErrorContains != "" {
				errors := parser.Errors
				require.True(t, len(errors) > 0, "error exist")
				last := errors[len(errors)-1]
				require.True(t, last.Field == c.QueryParam, fmt.Sprintf("query param %q did not fail", c.QueryParam))
				require.Contains(t, last.Detail, c.ExpectedErrorContains, "correct error")
			}
		})
	}
}
