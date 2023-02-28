package httpapi

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/coder/coder/codersdk"

	"golang.org/x/xerrors"
)

// QueryParamParser is a helper for parsing all query params and gathering all
// errors in 1 sweep. This means all invalid fields are returned at once,
// rather than only returning the first error
type QueryParamParser struct {
	// Errors is the set of errors to return via the API. If the length
	// of this set is 0, there are no errors!.
	Errors []codersdk.ValidationError
	// Parsed is a map of all query params that were parsed. This is useful
	// for checking if extra query params were passed in.
	Parsed map[string]bool
}

func NewQueryParamParser() *QueryParamParser {
	return &QueryParamParser{
		Errors: []codersdk.ValidationError{},
		Parsed: map[string]bool{},
	}
}

// ErrorExcessParams checks if any query params were passed in that were not
// parsed. If so, it adds an error to the parser as these values are not valid
// query parameters.
func (p *QueryParamParser) ErrorExcessParams(values url.Values) {
	for k := range values {
		if _, ok := p.Parsed[k]; !ok {
			p.Errors = append(p.Errors, codersdk.ValidationError{
				Field:  k,
				Detail: fmt.Sprintf("Query param %q is not a valid query param", k),
			})
		}
	}
}

func (p *QueryParamParser) addParsed(key string) {
	p.Parsed[key] = true
}

func (p *QueryParamParser) Int(vals url.Values, def int, queryParam string) int {
	v, err := parseQueryParam(p, vals, strconv.Atoi, def, queryParam)
	if err != nil {
		p.Errors = append(p.Errors, codersdk.ValidationError{
			Field:  queryParam,
			Detail: fmt.Sprintf("Query param %q must be a valid integer (%s)", queryParam, err.Error()),
		})
	}
	return v
}

func (p *QueryParamParser) UUIDorMe(vals url.Values, def uuid.UUID, me uuid.UUID, queryParam string) uuid.UUID {
	return ParseCustom(p, vals, def, queryParam, func(v string) (uuid.UUID, error) {
		if v == "me" {
			return me, nil
		}
		return uuid.Parse(v)
	})
}

func (p *QueryParamParser) UUID(vals url.Values, def uuid.UUID, queryParam string) uuid.UUID {
	v, err := parseQueryParam(p, vals, uuid.Parse, def, queryParam)
	if err != nil {
		p.Errors = append(p.Errors, codersdk.ValidationError{
			Field:  queryParam,
			Detail: fmt.Sprintf("Query param %q must be a valid uuid", queryParam),
		})
	}
	return v
}

func (p *QueryParamParser) UUIDs(vals url.Values, def []uuid.UUID, queryParam string) []uuid.UUID {
	v, err := parseQueryParam(p, vals, func(v string) ([]uuid.UUID, error) {
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
		p.Errors = append(p.Errors, codersdk.ValidationError{
			Field:  queryParam,
			Detail: fmt.Sprintf("Query param %q has invalid uuids: %q", queryParam, err.Error()),
		})
	}
	return v
}

func (p *QueryParamParser) String(vals url.Values, def string, queryParam string) string {
	v, _ := parseQueryParam(p, vals, func(v string) (string, error) {
		return v, nil
	}, def, queryParam)
	return v
}

func (p *QueryParamParser) Strings(vals url.Values, def []string, queryParam string) []string {
	v, _ := parseQueryParam(p, vals, func(v string) ([]string, error) {
		if v == "" {
			return []string{}, nil
		}
		return strings.Split(v, ","), nil
	}, def, queryParam)
	return v
}

// ParseCustom has to be a function, not a method on QueryParamParser because generics
// cannot be used on struct methods.
func ParseCustom[T any](parser *QueryParamParser, vals url.Values, def T, queryParam string, parseFunc func(v string) (T, error)) T {
	v, err := parseQueryParam(parser, vals, parseFunc, def, queryParam)
	if err != nil {
		parser.Errors = append(parser.Errors, codersdk.ValidationError{
			Field:  queryParam,
			Detail: fmt.Sprintf("Query param %q has invalid value: %s", queryParam, err.Error()),
		})
	}
	return v
}

func parseQueryParam[T any](parser *QueryParamParser, vals url.Values, parse func(v string) (T, error), def T, queryParam string) (T, error) {
	parser.addParsed(queryParam)
	if !vals.Has(queryParam) || vals.Get(queryParam) == "" {
		return def, nil
	}
	str := vals.Get(queryParam)
	return parse(str)
}
