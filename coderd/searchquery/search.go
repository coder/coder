package searchquery

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/coder/coder/coderd/httpapi"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"

	"github.com/coder/coder/codersdk"
)

func Workspace(query string, page codersdk.Pagination, agentInactiveDisconnectTimeout time.Duration) (database.GetWorkspacesParams, []codersdk.ValidationError) {
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
			values.Set("name", parts[0])
		case 2:
			values.Set("owner", parts[0])
			values.Set("name", parts[1])
		default:
			return xerrors.Errorf("Query element %q can only contain 1 '/'", term)
		}
		return nil
	})
	if errors != nil {
		return filter, errors
	}

	parser := httpapi.NewQueryParamParser()
	filter.OwnerUsername = parser.String(values, "", "owner")
	filter.TemplateName = parser.String(values, "", "template")
	filter.Name = parser.String(values, "", "name")
	filter.Status = parser.String(values, "", "status")
	filter.HasAgent = parser.String(values, "", "has-agent")
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
			searchValues.Set(strings.ToLower(parts[0]), parts[1])
		default:
			return nil, []codersdk.ValidationError{
				{
					Field:  "q",
					Detail: fmt.Sprintf("Query element %q can only contain 1 ':'", element),
				},
			}
		}
	}
	return searchValues, nil
}
