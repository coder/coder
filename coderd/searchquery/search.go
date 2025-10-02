// Package searchquery provides a unified search interface for Coder entities.
//
// The package parses human-readable search queries into structured database
// parameters that can be used to filter database queries efficiently.
//
// # Search Query Format
//
// The package supports two types of search terms:
//
// 1. Key-Value Pairs (with colon):
//   - "owner:prebuilds" → filters by owner username
//   - "template:my-template" → filters by template name
//   - "status:running" → filters by status
//
// 2. Free-form Terms (without colon):
//   - "my workspace" → uses entity-specific default search field
//   - For workspaces: searches by workspace name
//   - For templates: searches by template display name
//   - For provisioner jobs: searches by job type
//
// # Query Processing
//
// The searchTerms() function is the core parser that:
//   - Splits queries by spaces while preserving quoted strings
//   - Groups non-field terms together for free-form search
//   - Validates query syntax and returns clear error messages
//   - Converts parsed terms into url.Values for further processing
//
// Each entity type (Workspaces, Templates, ProvisionerJobs, etc.) has its
// own search function that:
//   - Uses the generic searchTerms() parser
//   - Maps search parameters to database filter structs
//   - Handles entity-specific logic (like "me" → current user)
//   - Returns database parameters for SQL queries
//
// # Performance
//
// The searchquery package only parses queries and generates database parameters.
// No in-memory filtering is performed.
//
// Example Usage
//
//	// Parse workspace search
//	filter, errors := searchquery.Workspaces(ctx, db, "owner:prebuilds template:my-template", page, timeout)
//	if len(errors) > 0 {
//	    return errors
//	}
//
//	// Use filter in database query
//	workspaces, err := db.GetWorkspaces(ctx, filter)
package searchquery

import (
	"context"
	"database/sql"
	"encoding/json"
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

// AuditLogs requires the database to fetch an organization by name
// to convert to organization uuid.
//
// Supported query parameters:
//
//   - request_id: UUID (can be used to search for associated audits e.g. connect/disconnect or open/close)
//   - resource_id: UUID
//   - resource_target: string
//   - username: string
//   - email: string
//   - date_from: string (date in format "2006-01-02")
//   - date_to: string (date in format "2006-01-02")
//   - organization: string (organization UUID or name)
//   - resource_type: string (enum)
//   - action: string (enum)
//   - build_reason: string (enum)
func AuditLogs(ctx context.Context, db database.Store, query string) (database.GetAuditLogsOffsetParams,
	database.CountAuditLogsParams, []codersdk.ValidationError,
) {
	values, errors := searchTerms(query, func(term string, values url.Values) error {
		values.Add("resource_type", term)
		return nil
	})
	if len(errors) > 0 {
		// nolint:exhaustruct // We don't need to initialize these structs because we return an error.
		return database.GetAuditLogsOffsetParams{}, database.CountAuditLogsParams{}, errors
	}

	const dateLayout = "2006-01-02"
	parser := httpapi.NewQueryParamParser()
	filter := database.GetAuditLogsOffsetParams{
		RequestID:      parser.UUID(values, uuid.Nil, "request_id"),
		ResourceID:     parser.UUID(values, uuid.Nil, "resource_id"),
		ResourceTarget: parser.String(values, "", "resource_target"),
		Username:       parser.String(values, "", "username"),
		Email:          parser.String(values, "", "email"),
		DateFrom:       parser.Time(values, time.Time{}, "date_from", dateLayout),
		DateTo:         parser.Time(values, time.Time{}, "date_to", dateLayout),
		OrganizationID: parseOrganization(ctx, db, parser, values, "organization"),
		ResourceType:   string(httpapi.ParseCustom(parser, values, "", "resource_type", httpapi.ParseEnum[database.ResourceType])),
		Action:         string(httpapi.ParseCustom(parser, values, "", "action", httpapi.ParseEnum[database.AuditAction])),
		BuildReason:    string(httpapi.ParseCustom(parser, values, "", "build_reason", httpapi.ParseEnum[database.BuildReason])),
	}
	if !filter.DateTo.IsZero() {
		filter.DateTo = filter.DateTo.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
	}

	// Prepare the count filter, which uses the same parameters as the GetAuditLogsOffsetParams.
	// nolint:exhaustruct // UserID is not obtained from the query parameters.
	countFilter := database.CountAuditLogsParams{
		RequestID:      filter.RequestID,
		ResourceID:     filter.ResourceID,
		ResourceTarget: filter.ResourceTarget,
		Username:       filter.Username,
		Email:          filter.Email,
		DateFrom:       filter.DateFrom,
		DateTo:         filter.DateTo,
		OrganizationID: filter.OrganizationID,
		ResourceType:   filter.ResourceType,
		Action:         filter.Action,
		BuildReason:    filter.BuildReason,
	}

	parser.ErrorExcessParams(values)
	return filter, countFilter, parser.Errors
}

func ConnectionLogs(ctx context.Context, db database.Store, query string, apiKey database.APIKey) (database.GetConnectionLogsOffsetParams, database.CountConnectionLogsParams, []codersdk.ValidationError) {
	values, errors := searchTerms(query, func(term string, values url.Values) error {
		values.Add("search", term)
		return nil
	})
	if len(errors) > 0 {
		// nolint:exhaustruct // We don't need to initialize these structs because we return an error.
		return database.GetConnectionLogsOffsetParams{}, database.CountConnectionLogsParams{}, errors
	}

	parser := httpapi.NewQueryParamParser()
	filter := database.GetConnectionLogsOffsetParams{
		OrganizationID:      parseOrganization(ctx, db, parser, values, "organization"),
		WorkspaceOwner:      parser.String(values, "", "workspace_owner"),
		WorkspaceOwnerEmail: parser.String(values, "", "workspace_owner_email"),
		Type:                string(httpapi.ParseCustom(parser, values, "", "type", httpapi.ParseEnum[database.ConnectionType])),
		Username:            parser.String(values, "", "username"),
		UserEmail:           parser.String(values, "", "user_email"),
		ConnectedAfter:      parser.Time3339Nano(values, time.Time{}, "connected_after"),
		ConnectedBefore:     parser.Time3339Nano(values, time.Time{}, "connected_before"),
		WorkspaceID:         parser.UUID(values, uuid.Nil, "workspace_id"),
		ConnectionID:        parser.UUID(values, uuid.Nil, "connection_id"),
		Status:              string(httpapi.ParseCustom(parser, values, "", "status", httpapi.ParseEnum[codersdk.ConnectionLogStatus])),
	}

	if filter.Username == "me" {
		filter.UserID = apiKey.UserID
		filter.Username = ""
	}

	if filter.WorkspaceOwner == "me" {
		filter.WorkspaceOwnerID = apiKey.UserID
		filter.WorkspaceOwner = ""
	}

	// This MUST be kept in sync with the above
	countFilter := database.CountConnectionLogsParams{
		OrganizationID:      filter.OrganizationID,
		WorkspaceOwner:      filter.WorkspaceOwner,
		WorkspaceOwnerID:    filter.WorkspaceOwnerID,
		WorkspaceOwnerEmail: filter.WorkspaceOwnerEmail,
		Type:                filter.Type,
		UserID:              filter.UserID,
		Username:            filter.Username,
		UserEmail:           filter.UserEmail,
		ConnectedAfter:      filter.ConnectedAfter,
		ConnectedBefore:     filter.ConnectedBefore,
		WorkspaceID:         filter.WorkspaceID,
		ConnectionID:        filter.ConnectionID,
		Status:              filter.Status,
	}
	parser.ErrorExcessParams(values)
	return filter, countFilter, parser.Errors
}

func Users(query string) (database.GetUsersParams, []codersdk.ValidationError) {
	values, errors := searchTerms(query, func(term string, values url.Values) error {
		values.Add("search", term)
		return nil
	})
	if len(errors) > 0 {
		return database.GetUsersParams{}, errors
	}

	parser := httpapi.NewQueryParamParser()
	filter := database.GetUsersParams{
		Search:          parser.String(values, "", "search"),
		Status:          httpapi.ParseCustomList(parser, values, []database.UserStatus{}, "status", httpapi.ParseEnum[database.UserStatus]),
		RbacRole:        parser.Strings(values, []string{}, "role"),
		LastSeenAfter:   parser.Time3339Nano(values, time.Time{}, "last_seen_after"),
		LastSeenBefore:  parser.Time3339Nano(values, time.Time{}, "last_seen_before"),
		CreatedAfter:    parser.Time3339Nano(values, time.Time{}, "created_after"),
		CreatedBefore:   parser.Time3339Nano(values, time.Time{}, "created_before"),
		GithubComUserID: parser.Int64(values, 0, "github_com_user_id"),
		LoginType:       httpapi.ParseCustomList(parser, values, []database.LoginType{}, "login_type", httpapi.ParseEnum[database.LoginType]),
	}
	parser.ErrorExcessParams(values)
	return filter, parser.Errors
}

func Workspaces(ctx context.Context, db database.Store, query string, page codersdk.Pagination, agentInactiveDisconnectTimeout time.Duration) (database.GetWorkspacesParams, []codersdk.ValidationError) {
	filter := database.GetWorkspacesParams{
		AgentInactiveDisconnectTimeoutSeconds: int64(agentInactiveDisconnectTimeout.Seconds()),

		// #nosec G115 - Safe conversion for pagination offset which is expected to be within int32 range
		Offset: int32(page.Offset),
		// #nosec G115 - Safe conversion for pagination limit which is expected to be within int32 range
		Limit: int32(page.Limit),
	}

	if query == "" {
		return filter, nil
	}

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
	filter.WorkspaceIds = parser.UUIDs(values, []uuid.UUID{}, "id")
	filter.OwnerUsername = parser.String(values, "", "owner")
	filter.TemplateName = parser.String(values, "", "template")
	filter.Name = parser.String(values, "", "name")
	filter.Status = string(httpapi.ParseCustom(parser, values, "", "status", httpapi.ParseEnum[database.WorkspaceStatus]))
	filter.HasAgent = parser.String(values, "", "has-agent")
	filter.Dormant = parser.Boolean(values, false, "dormant")
	filter.LastUsedAfter = parser.Time3339Nano(values, time.Time{}, "last_used_after")
	filter.LastUsedBefore = parser.Time3339Nano(values, time.Time{}, "last_used_before")
	filter.UsingActive = sql.NullBool{
		// Invert the value of the query parameter to get the correct value.
		// UsingActive returns if the workspace is on the latest template active version.
		Bool: !parser.Boolean(values, true, "outdated"),
		// Only include this search term if it was provided. Otherwise default to omitting it
		// which will return all workspaces.
		Valid: values.Has("outdated"),
	}
	filter.HasAITask = parser.NullableBoolean(values, sql.NullBool{}, "has-ai-task")
	filter.HasExternalAgent = parser.NullableBoolean(values, sql.NullBool{}, "has_external_agent")
	filter.OrganizationID = parseOrganization(ctx, db, parser, values, "organization")
	filter.Shared = parser.NullableBoolean(values, sql.NullBool{}, "shared")
	// TODO: support "me" by passing in the actorID
	filter.SharedWithUserID = parseUser(ctx, db, parser, values, "shared_with_user", uuid.Nil)
	filter.SharedWithGroupID = parseGroup(ctx, db, parser, values, "shared_with_group")

	type paramMatch struct {
		name  string
		value *string
	}
	// parameter matching takes the form of:
	//	`param:<name>[=<value>]`
	// If the value is omitted, then we match on the presence of the parameter.
	// If the value is provided, then we match on the parameter and value.
	params := httpapi.ParseCustomList(parser, values, []paramMatch{}, "param", func(v string) (paramMatch, error) {
		// Ignore excess spaces
		v = strings.TrimSpace(v)
		parts := strings.Split(v, "=")
		if len(parts) == 1 {
			// Only match on the presence of the parameter
			return paramMatch{name: parts[0], value: nil}, nil
		}
		if len(parts) == 2 {
			if parts[1] == "" {
				return paramMatch{}, xerrors.Errorf("query element %q has an empty value. omit the '=' to match just on the parameter name", v)
			}
			// Match on the parameter and value
			return paramMatch{name: parts[0], value: &parts[1]}, nil
		}
		return paramMatch{}, xerrors.Errorf("query element %q can only contain 1 '='", v)
	})
	for _, p := range params {
		if p.value == nil {
			filter.HasParam = append(filter.HasParam, p.name)
			continue
		}
		filter.ParamNames = append(filter.ParamNames, p.name)
		filter.ParamValues = append(filter.ParamValues, *p.value)
	}

	parser.ErrorExcessParams(values)
	return filter, parser.Errors
}

func Templates(ctx context.Context, db database.Store, actorID uuid.UUID, query string) (database.GetTemplatesWithFilterParams, []codersdk.ValidationError) {
	values, errors := searchTerms(query, func(term string, values url.Values) error {
		// Default to the display name
		values.Add("display_name", term)
		return nil
	})
	if len(errors) > 0 {
		return database.GetTemplatesWithFilterParams{}, errors
	}

	parser := httpapi.NewQueryParamParser()
	filter := database.GetTemplatesWithFilterParams{
		Deleted:          parser.Boolean(values, false, "deleted"),
		OrganizationID:   parseOrganization(ctx, db, parser, values, "organization"),
		ExactName:        parser.String(values, "", "exact_name"),
		ExactDisplayName: parser.String(values, "", "exact_display_name"),
		FuzzyName:        parser.String(values, "", "name"),
		FuzzyDisplayName: parser.String(values, "", "display_name"),
		IDs:              parser.UUIDs(values, []uuid.UUID{}, "ids"),
		Deprecated:       parser.NullableBoolean(values, sql.NullBool{}, "deprecated"),
		HasAITask:        parser.NullableBoolean(values, sql.NullBool{}, "has-ai-task"),
		AuthorID:         parser.UUID(values, uuid.Nil, "author_id"),
		AuthorUsername:   parser.String(values, "", "author"),
		HasExternalAgent: parser.NullableBoolean(values, sql.NullBool{}, "has_external_agent"),
	}

	if filter.AuthorUsername == codersdk.Me {
		filter.AuthorID = actorID
		filter.AuthorUsername = ""
	}

	parser.ErrorExcessParams(values)
	return filter, parser.Errors
}

func AIBridgeInterceptions(ctx context.Context, db database.Store, query string, page codersdk.Pagination, actorID uuid.UUID) (database.ListAIBridgeInterceptionsParams, []codersdk.ValidationError) {
	// nolint:exhaustruct // Empty values just means "don't filter by that field".
	filter := database.ListAIBridgeInterceptionsParams{
		AfterID: page.AfterID,
		// #nosec G115 - Safe conversion for pagination limit which is expected to be within int32 range
		Limit: int32(page.Limit),
	}

	if query == "" {
		return filter, nil
	}

	values, errors := searchTerms(query, func(term string, values url.Values) error {
		// Default to the initiating user
		values.Add("user", term)
		return nil
	})
	if len(errors) > 0 {
		return filter, errors
	}

	parser := httpapi.NewQueryParamParser()
	filter.InitiatorID = parseUser(ctx, db, parser, values, "initiator", actorID)
	filter.Provider = parser.String(values, "", "provider")
	filter.Model = parser.String(values, "", "model")

	// Time must be between started_after and started_before.
	filter.StartedAfter = parser.Time3339Nano(values, time.Time{}, "started_after")
	filter.StartedBefore = parser.Time3339Nano(values, time.Time{}, "started_before")
	if !filter.StartedBefore.IsZero() && !filter.StartedAfter.IsZero() && !filter.StartedBefore.After(filter.StartedAfter) {
		parser.Errors = append(parser.Errors, codersdk.ValidationError{
			Field:  "started_before",
			Detail: `Query param "started_before" has invalid value: "started_before" must be after "started_after" if set`,
		})
	}

	parser.ErrorExcessParams(values)
	return filter, parser.Errors
}

// searchTerms parses a search query string into structured key-value pairs.
//
// It handles two types of search terms:
//   - Key-value pairs: "owner:prebuilds" → {"owner": "prebuilds"}
//   - Free-form terms: "my workspace" → calls defaultKey("my workspace", values)
//
// The function uses a two-pass parsing approach:
//  1. Split by spaces while preserving quoted strings
//  2. Group non-field terms together for free-form search
//
// Parameters:
//   - query: The search query string to parse
//   - defaultKey: Function called for terms without colons to determine default field
//
// Returns:
//   - url.Values: Parsed key-value pairs
//   - []codersdk.ValidationError: Any parsing errors encountered
//
// Example:
//
//	searchTerms("owner:prebuilds template:my-template", func(term, values) {
//	    values.Add("name", term) // Default to searching by name
//	    return nil
//	})
//	// Returns: {"owner": ["prebuilds"], "template": ["my-template"]}
func searchTerms(query string, defaultKey func(term string, values url.Values) error) (url.Values, []codersdk.ValidationError) {
	// Always lowercase for all searches.
	query = strings.ToLower(query)
	searchValues := make(url.Values)

	// Because we do this in 2 passes, we want to maintain quotes on the first
	// pass. Further splitting occurs on the second pass and quotes will be
	// dropped.
	tokens := splitQueryParameterByDelimiter(query, ' ', true)
	elements := processTokens(tokens)
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

	return searchValues, nil
}

func parseOrganization(ctx context.Context, db database.Store, parser *httpapi.QueryParamParser, vals url.Values, queryParam string) uuid.UUID {
	return httpapi.ParseCustom(parser, vals, uuid.Nil, queryParam, func(v string) (uuid.UUID, error) {
		if v == "" {
			return uuid.Nil, nil
		}
		organizationID, err := uuid.Parse(v)
		if err == nil {
			return organizationID, nil
		}
		organization, err := db.GetOrganizationByName(ctx, database.GetOrganizationByNameParams{
			Name: v, Deleted: false,
		})
		if err != nil {
			return uuid.Nil, xerrors.Errorf("organization %q either does not exist, or you are unauthorized to view it", v)
		}
		return organization.ID, nil
	})
}

func parseUser(ctx context.Context, db database.Store, parser *httpapi.QueryParamParser, vals url.Values, queryParam string, actorID uuid.UUID) uuid.UUID {
	return httpapi.ParseCustom(parser, vals, uuid.Nil, queryParam, func(v string) (uuid.UUID, error) {
		if v == "" {
			return uuid.Nil, nil
		}
		if v == codersdk.Me && actorID != uuid.Nil {
			return actorID, nil
		}
		userID, err := uuid.Parse(v)
		if err == nil {
			return userID, nil
		}
		user, err := db.GetUserByEmailOrUsername(ctx, database.GetUserByEmailOrUsernameParams{
			Username: v,
		})
		if err != nil {
			return uuid.Nil, xerrors.Errorf("user %q either does not exist, or you are unauthorized to view them", v)
		}
		return user.ID, nil
	})
}

// Parse a group filter value into a group UUID.
// Supported formats:
//   - <group-uuid>
//   - <organization-name>/<group-name>
//   - <group-name> (resolved in the default organization)
func parseGroup(ctx context.Context, db database.Store, parser *httpapi.QueryParamParser, vals url.Values, queryParam string) uuid.UUID {
	return httpapi.ParseCustom(parser, vals, uuid.Nil, queryParam, func(v string) (uuid.UUID, error) {
		if v == "" {
			return uuid.Nil, nil
		}
		groupID, err := uuid.Parse(v)
		if err == nil {
			return groupID, nil
		}

		var groupName string
		var org database.Organization
		parts := strings.Split(v, "/")
		switch len(parts) {
		case 1:
			dbOrg, err := db.GetDefaultOrganization(ctx)
			if err != nil {
				return uuid.Nil, xerrors.New("fetching default organization")
			}
			org = dbOrg
			groupName = parts[0]
		case 2:
			orgName := parts[0]
			if err := codersdk.NameValid(orgName); err != nil {
				return uuid.Nil, xerrors.Errorf("invalid organization name %w", err)
			}
			dbOrg, err := db.GetOrganizationByName(ctx, database.GetOrganizationByNameParams{
				Name: orgName,
			})
			if err != nil {
				return uuid.Nil, xerrors.Errorf("organization %q either does not exist, or you are unauthorized to view it", orgName)
			}
			org = dbOrg

			groupName = parts[1]

		default:
			return uuid.Nil, xerrors.New("invalid organization or group name, the filter must be in the pattern of <organization name>/<group name>")
		}

		if err := codersdk.GroupNameValid(groupName); err != nil {
			return uuid.Nil, xerrors.Errorf("invalid group name %w", err)
		}

		group, err := db.GetGroupByOrgAndName(ctx, database.GetGroupByOrgAndNameParams{
			OrganizationID: org.ID,
			Name:           groupName,
		})
		if err != nil {
			return uuid.Nil, xerrors.Errorf("group %q either does not exist, does not belong to the organization %q, or you are unauthorized to view it", groupName, org.Name)
		}
		return group.ID, nil
	})
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

// processTokens takes the split tokens and groups them based on a delimiter (':').
// Tokens without a delimiter present are joined to support searching with spaces.
//
//	Example Input: ['deprecated:false', 'test', 'template']
//	Example Output: ['deprecated:false', 'test template']
func processTokens(tokens []string) []string {
	var results []string
	var nonFieldTerms []string
	for _, token := range tokens {
		if strings.Contains(token, string(':')) {
			results = append(results, token)
		} else {
			nonFieldTerms = append(nonFieldTerms, token)
		}
	}
	if len(nonFieldTerms) > 0 {
		results = append(results, strings.Join(nonFieldTerms, " "))
	}
	return results
}

// ProvisionerJobs parses a search query for provisioner jobs and returns database filter parameters.
//
// Supported search parameters:
//   - status:<status> - Filter by job status (pending, running, succeeded, failed, etc.)
//   - initiator:<user> - Filter by user who initiated the job
//   - organization:<org> - Filter by organization
//   - tags:<json> - Filter by job tags (JSON format)
//
// Free-form terms (without colons) default to searching by job type.
//
// All filtering is performed in SQL using database indexes for optimal performance.
//
// Example queries:
//   - "status:running initiator:me"
//   - "status:pending status:running" (multiple statuses)
//   - "workspace_build" (searches by job type)
func ProvisionerJobs(ctx context.Context, db database.Store, query string, page codersdk.Pagination) (database.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisionerParams, []codersdk.ValidationError) {
	filter := database.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisionerParams{
		// #nosec G115 - Safe conversion for pagination limit which is expected to be within int32 range
		Limit: sql.NullInt32{Int32: int32(page.Limit), Valid: page.Limit > 0},
	}

	if query == "" {
		return filter, nil
	}

	values, errors := searchTerms(query, func(_ string, _ url.Values) error {
		// Provisioner jobs don't support free-form search terms
		// Users must specify search parameters like status:, initiator:, etc.
		return xerrors.Errorf("Free-form search terms are not supported for provisioner jobs. Use specific search parameters like 'status:running', 'initiator:username', or 'organization:orgname'")
	})
	if len(errors) > 0 {
		return filter, errors
	}

	parser := httpapi.NewQueryParamParser()
	filter.OrganizationID = parseOrganization(ctx, db, parser, values, "organization")
	filter.Status = httpapi.ParseCustomList(parser, values, []database.ProvisionerJobStatus{}, "status", httpapi.ParseEnum[database.ProvisionerJobStatus])
	filter.InitiatorID = parseUser(ctx, db, parser, values, "initiator", uuid.Nil)

	// Parse tags as a map
	tagsStr := parser.String(values, "", "tags")
	if tagsStr != "" {
		var tags map[string]string
		if err := json.Unmarshal([]byte(tagsStr), &tags); err == nil {
			filter.Tags = database.StringMap(tags)
		}
	}

	parser.ErrorExcessParams(values)
	return filter, parser.Errors
}
