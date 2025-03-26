package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
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
	// RequiredNotEmptyParams is a map of all query params that are required. This is useful
	// for forcing a value to be provided.
	RequiredNotEmptyParams map[string]bool
}

func NewQueryParamParser() *QueryParamParser {
	return &QueryParamParser{
		Errors:                 []codersdk.ValidationError{},
		Parsed:                 map[string]bool{},
		RequiredNotEmptyParams: map[string]bool{},
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
			Detail: fmt.Sprintf("Query param %q must be a valid positive integer: %s", queryParam, err.Error()),
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
			Detail: fmt.Sprintf("Query param %q must be a valid integer: %s", queryParam, err.Error()),
		})
	}
	return v
}

func (p *QueryParamParser) Int64(vals url.Values, def int64, queryParam string) int64 {
	v, err := parseQueryParam(p, vals, func(v string) (int64, error) {
		return strconv.ParseInt(v, 10, 64)
	}, def, queryParam)
	if err != nil {
		p.Errors = append(p.Errors, codersdk.ValidationError{
			Field:  queryParam,
			Detail: fmt.Sprintf("Query param %q must be a valid 64-bit integer: %s", queryParam, err.Error()),
		})
		return 0
	}
	return v
}

// PositiveInt32 function checks if the given value is 32-bit and positive.
//
// We can't use `uint32` as the value must be within the range  <0,2147483647>
// as database expects it. Otherwise, the database query fails with `pq: OFFSET must not be negative`.
func (p *QueryParamParser) PositiveInt32(vals url.Values, def int32, queryParam string) int32 {
	v, err := parseQueryParam(p, vals, func(v string) (int32, error) {
		intValue, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			return 0, err
		}
		if intValue < 0 {
			return 0, xerrors.Errorf("value is negative")
		}
		return int32(intValue), nil
	}, def, queryParam)
	if err != nil {
		p.Errors = append(p.Errors, codersdk.ValidationError{
			Field:  queryParam,
			Detail: fmt.Sprintf("Query param %q must be a valid 32-bit positive integer: %s", queryParam, err.Error()),
		})
	}
	return v
}

// NullableBoolean will return a null sql value if no input is provided.
// SQLc still uses sql.NullBool rather than the generic type. So converting from
// the generic type is required.
func (p *QueryParamParser) NullableBoolean(vals url.Values, def sql.NullBool, queryParam string) sql.NullBool {
	v, err := parseNullableQueryParam[bool](p, vals, strconv.ParseBool, sql.Null[bool]{
		V:     def.Bool,
		Valid: def.Valid,
	}, queryParam)
	if err != nil {
		p.Errors = append(p.Errors, codersdk.ValidationError{
			Field:  queryParam,
			Detail: fmt.Sprintf("Query param %q must be a valid boolean: %s", queryParam, err.Error()),
		})
	}

	return sql.NullBool{
		Bool:  v.V,
		Valid: v.Valid,
	}
}

func (p *QueryParamParser) Boolean(vals url.Values, def bool, queryParam string) bool {
	v, err := parseQueryParam(p, vals, strconv.ParseBool, def, queryParam)
	if err != nil {
		p.Errors = append(p.Errors, codersdk.ValidationError{
			Field:  queryParam,
			Detail: fmt.Sprintf("Query param %q must be a valid boolean: %s", queryParam, err.Error()),
		})
	}
	return v
}

func (p *QueryParamParser) RequiredNotEmpty(queryParam ...string) *QueryParamParser {
	for _, q := range queryParam {
		p.RequiredNotEmptyParams[q] = true
	}
	return p
}

// UUIDorName will parse a string as a UUID, if it fails, it uses the "fetchByName"
// function to return a UUID based on the value as a string.
// This is useful when fetching something like an organization by ID or by name.
func (p *QueryParamParser) UUIDorName(vals url.Values, def uuid.UUID, queryParam string, fetchByName func(name string) (uuid.UUID, error)) uuid.UUID {
	return ParseCustom(p, vals, def, queryParam, func(v string) (uuid.UUID, error) {
		id, err := uuid.Parse(v)
		if err == nil {
			return id, nil
		}
		return fetchByName(v)
	})
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

func (p *QueryParamParser) RedirectURL(vals url.Values, base *url.URL, queryParam string) *url.URL {
	v, err := parseQueryParam(p, vals, url.Parse, base, queryParam)
	if err != nil {
		p.Errors = append(p.Errors, codersdk.ValidationError{
			Field:  queryParam,
			Detail: fmt.Sprintf("Query param %q must be a valid url: %s", queryParam, err.Error()),
		})
	}

	// It can be a sub-directory but not a sub-domain, as we have apps on
	// sub-domains and that seems too dangerous.
	if v.Host != base.Host || !strings.HasPrefix(v.Path, base.Path) {
		p.Errors = append(p.Errors, codersdk.ValidationError{
			Field:  queryParam,
			Detail: fmt.Sprintf("Query param %q must be a subset of %s", queryParam, base),
		})
	}

	return v
}

func (p *QueryParamParser) Time(vals url.Values, def time.Time, queryParam, layout string) time.Time {
	return p.timeWithMutate(vals, def, queryParam, layout, nil)
}

// Time uses the default time format of RFC3339Nano and always returns a UTC time.
func (p *QueryParamParser) Time3339Nano(vals url.Values, def time.Time, queryParam string) time.Time {
	layout := time.RFC3339Nano
	// All search queries are forced to lowercase. But the RFC format requires
	// upper case letters. So just uppercase the term.
	return p.timeWithMutate(vals, def, queryParam, layout, strings.ToUpper)
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
	v, err := parseQueryParam(p, vals, func(v string) (string, error) {
		return v, nil
	}, def, queryParam)
	if err != nil {
		p.Errors = append(p.Errors, codersdk.ValidationError{
			Field:  queryParam,
			Detail: fmt.Sprintf("Query param %q must be a valid string: %s", queryParam, err.Error()),
		})
	}
	return v
}

func (p *QueryParamParser) Strings(vals url.Values, def []string, queryParam string) []string {
	return ParseCustomList(p, vals, def, queryParam, func(v string) (string, error) {
		return v, nil
	})
}

func (p *QueryParamParser) JSONStringMap(vals url.Values, def map[string]string, queryParam string) map[string]string {
	v, err := parseQueryParam(p, vals, func(v string) (map[string]string, error) {
		var m map[string]string
		if err := json.NewDecoder(strings.NewReader(v)).Decode(&m); err != nil {
			return nil, err
		}
		return m, nil
	}, def, queryParam)
	if err != nil {
		p.Errors = append(p.Errors, codersdk.ValidationError{
			Field:  queryParam,
			Detail: fmt.Sprintf("Query param %q must be a valid JSON object: %s", queryParam, err.Error()),
		})
	}
	return v
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

// ParseCustomList is a function that handles csv query params or multiple values
// for a query param.
// Csv is supported as it is a common way to pass multiple values in a query param.
// Multiple values is supported (key=value&key=value2) for feature parity with GitHub issue search.
func ParseCustomList[T any](parser *QueryParamParser, vals url.Values, def []T, queryParam string, parseFunc func(v string) (T, error)) []T {
	v, err := parseQueryParamSet(parser, vals, func(set []string) ([]T, error) {
		// Gather all terms.
		allTerms := make([]string, 0, len(set))
		for _, s := range set {
			// If a term is a csv, break it out into individual terms.
			terms := strings.Split(s, ",")
			allTerms = append(allTerms, terms...)
		}

		var badErrors error
		var output []T
		for _, s := range allTerms {
			good, err := parseFunc(s)
			if err != nil {
				badErrors = errors.Join(badErrors, err)
				continue
			}
			output = append(output, good)
		}
		if badErrors != nil {
			return []T{}, badErrors
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

func parseNullableQueryParam[T any](parser *QueryParamParser, vals url.Values, parse func(v string) (T, error), def sql.Null[T], queryParam string) (sql.Null[T], error) {
	setParse := parseSingle(parser, parse, def.V, queryParam)
	return parseQueryParamSet[sql.Null[T]](parser, vals, func(set []string) (sql.Null[T], error) {
		if len(set) == 0 {
			return sql.Null[T]{
				Valid: false,
			}, nil
		}

		value, err := setParse(set)
		if err != nil {
			return sql.Null[T]{}, err
		}
		return sql.Null[T]{
			V:     value,
			Valid: true,
		}, nil
	}, def, queryParam)
}

// parseQueryParam expects just 1 value set for the given query param.
func parseQueryParam[T any](parser *QueryParamParser, vals url.Values, parse func(v string) (T, error), def T, queryParam string) (T, error) {
	setParse := parseSingle(parser, parse, def, queryParam)
	return parseQueryParamSet(parser, vals, setParse, def, queryParam)
}

func parseSingle[T any](parser *QueryParamParser, parse func(v string) (T, error), def T, queryParam string) func(set []string) (T, error) {
	return func(set []string) (T, error) {
		if len(set) > 1 {
			// Set as a parser.Error rather than return an error.
			// Returned errors are errors from the passed in `parse` function, and
			// imply the query param value had attempted to be parsed.
			// By raising the error this way, we can also more easily control how it
			// is presented to the user. A returned error is wrapped with more text.
			parser.Errors = append(parser.Errors, codersdk.ValidationError{
				Field:  queryParam,
				Detail: fmt.Sprintf("Query param %q provided more than once, found %d times. Only provide 1 instance of this query param.", queryParam, len(set)),
			})
			return def, nil
		}
		return parse(set[0])
	}
}

func parseQueryParamSet[T any](parser *QueryParamParser, vals url.Values, parse func(set []string) (T, error), def T, queryParam string) (T, error) {
	parser.addParsed(queryParam)
	// If the query param is required and not present, return an error.
	if parser.RequiredNotEmptyParams[queryParam] && (!vals.Has(queryParam) || vals.Get(queryParam) == "") {
		parser.Errors = append(parser.Errors, codersdk.ValidationError{
			Field:  queryParam,
			Detail: fmt.Sprintf("Query param %q is required and cannot be empty", queryParam),
		})
		return def, nil
	}

	// If the query param is not present, return the default value.
	if !vals.Has(queryParam) || vals.Get(queryParam) == "" {
		return def, nil
	}

	return parse(vals[queryParam])
}
