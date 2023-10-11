package searchquery

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

func AuditLogs(query string) (database.GetAuditLogsOffsetParams, []codersdk.ValidationError) {
	// Always lowercase for all searches.
	query = strings.ToLower(query)
	values, errors := searchTerms(query, func(term string, values url.Values) error {
		values.Add("resource_type", term)
		return nil
	})
	if len(errors) > 0 {
		return database.GetAuditLogsOffsetParams{}, errors
	}

	const dateLayout = "2006-01-02"
	parser := httpapi.NewQueryParamParser()
	filter := database.GetAuditLogsOffsetParams{
		ResourceID:     parser.UUID(values, uuid.Nil, "resource_id"),
		ResourceTarget: parser.String(values, "", "resource_target"),
		Username:       parser.String(values, "", "username"),
		Email:          parser.String(values, "", "email"),
		DateFrom:       parser.Time(values, time.Time{}, "date_from", dateLayout),
		DateTo:         parser.Time(values, time.Time{}, "date_to", dateLayout),
		ResourceType:   string(httpapi.ParseCustom(parser, values, "", "resource_type", httpapi.ParseEnum[database.ResourceType])),
		Action:         string(httpapi.ParseCustom(parser, values, "", "action", httpapi.ParseEnum[database.AuditAction])),
		BuildReason:    string(httpapi.ParseCustom(parser, values, "", "build_reason", httpapi.ParseEnum[database.BuildReason])),
	}
	if !filter.DateTo.IsZero() {
		filter.DateTo = filter.DateTo.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
	}
	parser.ErrorExcessParams(values)
	return filter, parser.Errors
}

func Users(query string) (database.GetUsersParams, []codersdk.ValidationError) {
	// Always lowercase for all searches.
	query = strings.ToLower(query)
	values, errors := searchTerms(query, func(term string, values url.Values) error {
		values.Add("search", term)
		return nil
	})
	if len(errors) > 0 {
		return database.GetUsersParams{}, errors
	}

	parser := httpapi.NewQueryParamParser()
	filter := database.GetUsersParams{
		Search:         parser.String(values, "", "search"),
		Status:         httpapi.ParseCustomList(parser, values, []database.UserStatus{}, "status", httpapi.ParseEnum[database.UserStatus]),
		RbacRole:       parser.Strings(values, []string{}, "role"),
		LastSeenAfter:  parser.Time3339Nano(values, time.Time{}, "last_seen_after"),
		LastSeenBefore: parser.Time3339Nano(values, time.Time{}, "last_seen_before"),
	}
	parser.ErrorExcessParams(values)
	return filter, parser.Errors
}

func Workspaces(query string, page codersdk.Pagination, agentInactiveDisconnectTimeout time.Duration) (database.GetWorkspacesParams, []codersdk.ValidationError) {
	filter := database.GetWorkspacesParams{
		AgentInactiveDisconnectTimeoutSeconds: int64(agentInactiveDisconnectTimeout.Seconds()),

		Offset: int32(page.Offset),
		Limit:  int32(page.Limit),
	}

	if query == "" {
		return filter, nil
	}

	// Always lowercase for all searches.
	query = strings.ToLower(query)
	values, errors := searchTerms(query, func(term string, values url.Values) error {
		// It is a workspace name, and maybe includes an owner
		parts := splitQueryParameterByDelimiter(term, '/', false)
		switch len(parts) {
		case 1:
			values.Add("name", parts[0])
		case 2:
			values.Add("owner", parts[0])
			values.Add("name", parts[1])
		default:
			return xerrors.Errorf("Query element %q can only contain 1 '/'", term)
		}
		return nil
	})
	if len(errors) > 0 {
		return filter, errors
	}

	parser := httpapi.NewQueryParamParser()
	filter.OwnerUsername = parser.String(values, "", "owner")
	filter.TemplateName = parser.String(values, "", "template")
	filter.Name = parser.String(values, "", "name")
	filter.Status = string(httpapi.ParseCustom(parser, values, "", "status", httpapi.ParseEnum[database.WorkspaceStatus]))
	filter.HasAgent = parser.String(values, "", "has-agent")
	filter.IsDormant = parser.String(values, "", "is-dormant")
	filter.LastUsedAfter = parser.Time3339Nano(values, time.Time{}, "last_used_after")
	filter.LastUsedBefore = parser.Time3339Nano(values, time.Time{}, "last_used_before")

	parser.ErrorExcessParams(values)
	return filter, parser.Errors
}

func searchTerms(query string, defaultKey func(term string, values url.Values) error) (url.Values, []codersdk.ValidationError) {
	searchValues := make(url.Values)

	// Because we do this in 2 passes, we want to maintain quotes on the first
	// pass. Further splitting occurs on the second pass and quotes will be
	// dropped.
	elements := splitQueryParameterByDelimiter(query, ' ', true)
	for _, element := range elements {
		if strings.HasPrefix(element, ":") || strings.HasSuffix(element, ":") {
			return nil, []codersdk.ValidationError{
				{
					Field:  "q",
					Detail: fmt.Sprintf("Query element %q cannot start or end with ':'", element),
				},
			}
		}
		parts := splitQueryParameterByDelimiter(element, ':', false)
		switch len(parts) {
		case 1:
			// No key:value pair. Use default behavior.
			err := defaultKey(element, searchValues)
			if err != nil {
				return nil, []codersdk.ValidationError{
					{Field: "q", Detail: err.Error()},
				}
			}
		case 2:
			searchValues.Add(strings.ToLower(parts[0]), parts[1])
		default:
			return nil, []codersdk.ValidationError{
				{
					Field:  "q",
					Detail: fmt.Sprintf("Query element %q can only contain 1 ':'", element),
				},
			}
		}
	}

	for k := range searchValues {
		if len(searchValues[k]) > 1 {
			return nil, []codersdk.ValidationError{
				{
					Field:  "q",
					Detail: fmt.Sprintf("Query parameter %q provided more than once, found %d times", k, len(searchValues[k])),
				},
			}
		}
	}

	return searchValues, nil
}

// splitQueryParameterByDelimiter takes a query string and splits it into the individual elements
// of the query. Each element is separated by a delimiter. All quoted strings are
// kept as a single element.
//
// Although all our names cannot have spaces, that is a validation error.
// We should still parse the quoted string as a single value so that validation
// can properly fail on the space. If we do not, a value of `template:"my name"`
// will search `template:"my name:name"`, which produces an empty list instead of
// an error.
// nolint:revive
func splitQueryParameterByDelimiter(query string, delimiter rune, maintainQuotes bool) []string {
	quoted := false
	parts := strings.FieldsFunc(query, func(r rune) bool {
		if r == '"' {
			quoted = !quoted
		}
		return !quoted && r == delimiter
	})
	if !maintainQuotes {
		for i, part := range parts {
			parts[i] = strings.Trim(part, "\"")
		}
	}

	return parts
}
