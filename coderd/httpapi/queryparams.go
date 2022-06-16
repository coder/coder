package httpapi

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"golang.org/x/xerrors"
)

// QueryParamParser is a helper for parsing all query params and gathering all
// errors in 1 sweep. This means all invalid fields are returned at once,
// rather than only returning the first error
type QueryParamParser struct {
	// Errors is the set of errors to return via the API. If the length
	// of this set is 0, there are no errors!.
	Errors []Error
}

func NewQueryParamParser() *QueryParamParser {
	return &QueryParamParser{
		Errors: []Error{},
	}
}

func (p *QueryParamParser) Int(vals url.Values, def int, queryParam string) int {
	v, err := parseQueryParam(vals, strconv.Atoi, def, queryParam)
	if err != nil {
		p.Errors = append(p.Errors, Error{
			Field:  queryParam,
			Detail: fmt.Sprintf("Query param %q must be a valid integer (%s)", queryParam, err.Error()),
		})
	}
	return v
}

func (p *QueryParamParser) UUIDorMe(vals url.Values, def uuid.UUID, me uuid.UUID, queryParam string) uuid.UUID {
	if vals.Get(queryParam) == "me" {
		return me
	}
	return p.UUID(vals, def, queryParam)
}

func (p *QueryParamParser) UUID(vals url.Values, def uuid.UUID, queryParam string) uuid.UUID {
	v, err := parseQueryParam(vals, uuid.Parse, def, queryParam)
	if err != nil {
		p.Errors = append(p.Errors, Error{
			Field:  queryParam,
			Detail: fmt.Sprintf("Query param %q must be a valid uuid", queryParam),
		})
	}
	return v
}

func (p *QueryParamParser) UUIDs(vals url.Values, def []uuid.UUID, queryParam string) []uuid.UUID {
	v, err := parseQueryParam(vals, func(v string) ([]uuid.UUID, error) {
		var badValues []string
		strs := strings.Split(v, ",")
		ids := make([]uuid.UUID, 0, len(strs))
		for _, s := range strs {
			id, err := uuid.Parse(strings.TrimSpace(s))
			if err != nil {
				badValues = append(badValues, v)
				continue
			}
			ids = append(ids, id)
		}

		if len(badValues) > 0 {
			return []uuid.UUID{}, xerrors.Errorf("%s", strings.Join(badValues, ","))
		}
		return ids, nil
	}, def, queryParam)
	if err != nil {
		p.Errors = append(p.Errors, Error{
			Field:  queryParam,
			Detail: fmt.Sprintf("Query param %q has invalid uuids: %q", queryParam, err.Error()),
		})
	}
	return v
}

func (p *QueryParamParser) String(vals url.Values, def string, queryParam string) string {
	v, err := parseQueryParam(vals, func(v string) (string, error) {
		return v, nil
	}, def, queryParam)
	if err != nil {
		p.Errors = append(p.Errors, Error{
			Field:  queryParam,
			Detail: fmt.Sprintf("Query param %q must be a valid string", queryParam),
		})
	}
	return v
}

func parseQueryParam[T any](vals url.Values, parse func(v string) (T, error), def T, queryParam string) (T, error) {
	if !vals.Has(queryParam) || vals.Get(queryParam) == "" {
		return def, nil
	}
	str := vals.Get(queryParam)
	return parse(str)
}
