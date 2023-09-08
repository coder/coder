package httpapi

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
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
	// RequiredParams is a map of all query params that are required. This is useful
	// for forcing a value to be provided.
	RequiredParams map[string]bool
}

func NewQueryParamParser() *QueryParamParser {
	return &QueryParamParser{
		Errors:         []codersdk.ValidationError{},
		Parsed:         map[string]bool{},
		RequiredParams: map[string]bool{},
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
				Detail: fmt.Sprintf("%q is not a valid query param", k),
			})
		}
	}
}

func (p *QueryParamParser) addParsed(key string) {
	p.Parsed[key] = true
}

func (p *QueryParamParser) UInt(vals url.Values, def uint64, queryParam string) uint64 {
	v, err := parseQueryParam(p, vals, func(v string) (uint64, error) {
		return strconv.ParseUint(v, 10, 64)
	}, def, queryParam)
	if err != nil {
		p.Errors = append(p.Errors, codersdk.ValidationError{
			Field:  queryParam,
			Detail: fmt.Sprintf("Query param %q must be a valid positive integer (%s)", queryParam, err.Error()),
		})
		return 0
	}
	return v
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

func (p *QueryParamParser) Required(queryParam string) *QueryParamParser {
	p.RequiredParams[queryParam] = true
	return p
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
	return ParseCustomList(p, vals, def, queryParam, func(v string) (uuid.UUID, error) {
		return uuid.Parse(strings.TrimSpace(v))
	})
}

func (p *QueryParamParser) Time(vals url.Values, def time.Time, queryParam, layout string) time.Time {
	return p.timeWithMutate(vals, def, queryParam, layout, nil)
}

// Time uses the default time format of RFC3339Nano and always returns a UTC time.
func (p *QueryParamParser) Time3339Nano(vals url.Values, def time.Time, queryParam string) time.Time {
	layout := time.RFC3339Nano
	return p.timeWithMutate(vals, def, queryParam, layout, func(term string) string {
		// All search queries are forced to lowercase. But the RFC format requires
		// upper case letters. So just uppercase the term.
		return strings.ToUpper(term)
	})
}

func (p *QueryParamParser) timeWithMutate(vals url.Values, def time.Time, queryParam, layout string, mutate func(term string) string) time.Time {
	v, err := parseQueryParam(p, vals, func(term string) (time.Time, error) {
		if mutate != nil {
			term = mutate(term)
		}
		t, err := time.Parse(layout, term)
		if err != nil {
			return time.Time{}, err
		}
		return t.UTC(), nil
	}, def, queryParam)
	if err != nil {
		p.Errors = append(p.Errors, codersdk.ValidationError{
			Field:  queryParam,
			Detail: fmt.Sprintf("Query param %q must be a valid date format (%s): %s", queryParam, layout, err.Error()),
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
	return ParseCustomList(p, vals, def, queryParam, func(v string) (string, error) {
		return v, nil
	})
}

// ValidEnum represents an enum that can be parsed and validated.
type ValidEnum interface {
	// Add more types as needed (avoid importing large dependency trees).
	~string

	// Valid is required on the enum type to be used with ParseEnum.
	Valid() bool
}

// ParseEnum is a function that can be passed into ParseCustom that handles enum
// validation.
func ParseEnum[T ValidEnum](term string) (T, error) {
	enum := T(term)
	if enum.Valid() {
		return enum, nil
	}
	var empty T
	return empty, xerrors.Errorf("%q is not a valid value", term)
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

// ParseCustomList is a function that handles csv query params.
func ParseCustomList[T any](parser *QueryParamParser, vals url.Values, def []T, queryParam string, parseFunc func(v string) (T, error)) []T {
	v, err := parseQueryParam(parser, vals, func(v string) ([]T, error) {
		terms := strings.Split(v, ",")
		var badValues []string
		var output []T
		for _, s := range terms {
			good, err := parseFunc(s)
			if err != nil {
				badValues = append(badValues, s)
				continue
			}
			output = append(output, good)
		}
		if len(badValues) > 0 {
			return []T{}, xerrors.Errorf("%s", strings.Join(badValues, ","))
		}

		return output, nil
	}, def, queryParam)
	if err != nil {
		parser.Errors = append(parser.Errors, codersdk.ValidationError{
			Field:  queryParam,
			Detail: fmt.Sprintf("Query param %q has invalid values: %s", queryParam, err.Error()),
		})
	}
	return v
}

func parseQueryParam[T any](parser *QueryParamParser, vals url.Values, parse func(v string) (T, error), def T, queryParam string) (T, error) {
	parser.addParsed(queryParam)
	// If the query param is required and not present, return an error.
	if parser.RequiredParams[queryParam] && (!vals.Has(queryParam)) {
		parser.Errors = append(parser.Errors, codersdk.ValidationError{
			Field:  queryParam,
			Detail: fmt.Sprintf("Query param %q is required", queryParam),
		})
		return def, nil
	}

	// If the query param is not present, return the default value.
	if !vals.Has(queryParam) || vals.Get(queryParam) == "" {
		return def, nil
	}

	str := vals.Get(queryParam)
	return parse(str)
}
