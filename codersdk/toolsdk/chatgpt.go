package toolsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/codersdk"
)

type ObjectType string

const (
	ObjectTypeTemplate  ObjectType = "template"
	ObjectTypeWorkspace ObjectType = "workspace"
)

type ObjectID struct {
	Type ObjectType
	ID   string
}

func (o ObjectID) String() string {
	return fmt.Sprintf("%s:%s", o.Type, o.ID)
}

func parseObjectID(id string) (ObjectID, error) {
	parts := strings.Split(id, ":")
	if len(parts) != 2 || (parts[0] != "template" && parts[0] != "workspace") {
		return ObjectID{}, xerrors.Errorf("invalid ID: %s", id)
	}
	return ObjectID{
		Type: ObjectType(parts[0]),
		ID:   parts[1],
	}, nil
}

func createObjectID(objectType ObjectType, id string) ObjectID {
	return ObjectID{
		Type: objectType,
		ID:   id,
	}
}

func searchTemplates(ctx context.Context, deps Deps, query string) ([]SearchResultItem, error) {
	serverURL := deps.ServerURL()
	templates, err := deps.coderClient.Templates(ctx, codersdk.TemplateFilter{
		SearchQuery: query,
	})
	if err != nil {
		return nil, err
	}
	results := make([]SearchResultItem, len(templates))
	for i, template := range templates {
		results[i] = SearchResultItem{
			ID:    createObjectID(ObjectTypeTemplate, template.ID.String()).String(),
			Title: template.DisplayName,
			Text:  template.Description,
			URL:   fmt.Sprintf("%s/templates/%s/%s", serverURL, template.OrganizationName, template.Name),
		}
	}
	return results, nil
}

func searchWorkspaces(ctx context.Context, deps Deps, query string) ([]SearchResultItem, error) {
	serverURL := deps.ServerURL()
	workspaces, err := deps.coderClient.Workspaces(ctx, codersdk.WorkspaceFilter{
		FilterQuery: query,
	})
	if err != nil {
		return nil, err
	}
	results := make([]SearchResultItem, len(workspaces.Workspaces))
	for i, workspace := range workspaces.Workspaces {
		results[i] = SearchResultItem{
			ID:    createObjectID(ObjectTypeWorkspace, workspace.ID.String()).String(),
			Title: workspace.Name,
			Text:  fmt.Sprintf("Owner: %s\nTemplate: %s\nLatest transition: %s", workspace.OwnerName, workspace.TemplateDisplayName, workspace.LatestBuild.Transition),
			URL:   fmt.Sprintf("%s/%s/%s", serverURL, workspace.OwnerName, workspace.Name),
		}
	}
	return results, nil
}

type SearchQueryType string

const (
	SearchQueryTypeTemplates  SearchQueryType = "templates"
	SearchQueryTypeWorkspaces SearchQueryType = "workspaces"
)

type SearchQuery struct {
	Type  SearchQueryType
	Query string
}

func parseSearchQuery(query string) (SearchQuery, error) {
	parts := strings.Split(query, "/")
	queryType := SearchQueryType(parts[0])
	if queryType != SearchQueryTypeTemplates && queryType != SearchQueryTypeWorkspaces {
		return SearchQuery{}, xerrors.Errorf("invalid query: %s", query)
	}
	queryString := ""
	if len(parts) > 1 {
		queryString = strings.Join(parts[1:], "/")
	}
	return SearchQuery{
		Type:  queryType,
		Query: queryString,
	}, nil
}

type SearchArgs struct {
	Query string `json:"query"`
}

type SearchResultItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Text  string `json:"text"`
	URL   string `json:"url"`
}

type SearchResult struct {
	Results []SearchResultItem `json:"results"`
}

// Implements the "search" tool as described in https://platform.openai.com/docs/mcp#search-tool.
// From my experiments with ChatGPT, it has access to the description that is provided in the
// tool definition. This is in contrast to the "fetch" tool, where ChatGPT does not have access
// to the description.
var ChatGPTSearch = Tool[SearchArgs, SearchResult]{
	Tool: aisdk.Tool{
		Name: ToolNameChatGPTSearch,
		// Note: the queries are passed directly to the list workspaces and list templates
		// endpoints. The list of accepted parameters below is not exhaustive - some are omitted
		// because they are not as useful in ChatGPT.
		Description: `Search for templates, workspaces, and files in workspaces.

To pick what you want to search for, use the following query formats:

- ` + "`" + `templates/<template-query>` + "`" + `: List templates. The query accepts the following, optional parameters delineated by whitespace:
	- "name:<name>" - Fuzzy search by template name (substring matching). Example: "name:docker"
	- "organization:<organization>" - Filter by organization ID or name. Example: "organization:coder"
	- "deprecated:<true|false>" - Filter by deprecated status. Example: "deprecated:true"
	- "deleted:<true|false>" - Filter by deleted status. Example: "deleted:true"
	- "has-ai-task:<true|false>" - Filter by whether the template has an AI task. Example: "has-ai-task:true"
- ` + "`" + `workspaces/<workspace-query>` + "`" + `: List workspaces. The query accepts the following, optional parameters delineated by whitespace:
	- "owner:<username>" - Filter by workspace owner (username or "me"). Example: "owner:alice" or "owner:me"
	- "template:<template-name>" - Filter by template name. Example: "template:web-development"
	- "name:<workspace-name>" - Filter by workspace name (substring matching). Example: "name:project"
	- "organization:<organization>" - Filter by organization ID or name. Example: "organization:engineering"
	- "status:<status>" - Filter by workspace/build status. Values: starting, stopping, deleting, deleted, stopped, started, running, pending, canceling, canceled, failed. Example: "status:running"
	- "has-agent:<agent-status>" - Filter by agent connectivity status. Values: connecting, connected, disconnected, timeout. Example: "has-agent:connected"
	- "dormant:<true|false>" - Filter dormant workspaces. Example: "dormant:true"
	- "outdated:<true|false>" - Filter workspaces using outdated template versions. Example: "outdated:true"
	- "last_used_after:<timestamp>" - Filter workspaces last used after a specific date. Example: "last_used_after:2023-12-01T00:00:00Z"
	- "last_used_before:<timestamp>" - Filter workspaces last used before a specific date. Example: "last_used_before:2023-12-31T23:59:59Z"
	- "has-ai-task:<true|false>" - Filter workspaces with AI tasks. Example: "has-ai-task:true"
	- "param:<name>" or "param:<name>=<value>" - Match workspaces by build parameters. Example: "param:environment=production" or "param:gpu"

# Examples

## Listing templates

List all templates without any filters.

` + "```" + `json
{
	"query": "templates"
}
` + "```" + `

List all templates with a "docker" substring in the name.

` + "```" + `json
{
	"query": "templates/name:docker"
}
` + "```" + `

List templates in a specific organization.

` + "```" + `json
{
	"query": "templates/organization:engineering"
}
` + "```" + `

List deprecated templates.

` + "```" + `json
{
	"query": "templates/deprecated:true"
}
` + "```" + `

List templates that have AI tasks.

` + "```" + `json
{
	"query": "templates/has-ai-task:true"
}
` + "```" + `

List templates with multiple filters - non-deprecated templates with "web" in the name.

` + "```" + `json
{
	"query": "templates/name:web deprecated:false"
}
` + "```" + `

List deleted templates (requires appropriate permissions).

` + "```" + `json
{
	"query": "templates/deleted:true"
}
` + "```" + `

## Listing workspaces

List all workspaces belonging to the current user.

` + "```" + `json
{
	"query": "workspaces/owner:me"
}
` + "```" + `

or 

` + "```" + `json
{
	"query": "workspaces"
}
` + "```" + `

List all workspaces belonging to a user with username "josh".

` + "```" + `json
{
	"query": "workspaces/owner:josh"
}
` + "```" + `

List all running workspaces.

` + "```" + `json
{
	"query": "workspaces/status:running"
}
` + "```" + `

List workspaces using a specific template.

` + "```" + `json
{
	"query": "workspaces/template:web-development"
}
` + "```" + `

List dormant workspaces.

` + "```" + `json
{
	"query": "workspaces/dormant:true"
}
` + "```" + `

List workspaces with connected agents.

` + "```" + `json
{
	"query": "workspaces/has-agent:connected"
}
` + "```" + `

List workspaces with multiple filters - running workspaces owned by "alice".

` + "```" + `json
{
	"query": "workspaces/owner:alice status:running"
}
` + "```" + `
`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"query": map[string]any{
					"type": "string",
				},
			},
			Required: []string{"query"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args SearchArgs) (SearchResult, error) {
		query, err := parseSearchQuery(args.Query)
		if err != nil {
			return SearchResult{}, err
		}
		switch query.Type {
		case SearchQueryTypeTemplates:
			results, err := searchTemplates(ctx, deps, query.Query)
			if err != nil {
				return SearchResult{}, err
			}
			return SearchResult{Results: results}, nil
		case SearchQueryTypeWorkspaces:
			searchQuery := query.Query
			if searchQuery == "" {
				searchQuery = "owner:me"
			}
			results, err := searchWorkspaces(ctx, deps, searchQuery)
			if err != nil {
				return SearchResult{}, err
			}
			return SearchResult{Results: results}, nil
		}
		return SearchResult{}, xerrors.Errorf("reached unreachable code with query: %s", args.Query)
	},
}

func fetchWorkspace(ctx context.Context, deps Deps, workspaceID string) (FetchResult, error) {
	parsedID, err := uuid.Parse(workspaceID)
	if err != nil {
		return FetchResult{}, xerrors.Errorf("invalid workspace ID, must be a valid UUID: %w", err)
	}
	workspace, err := deps.coderClient.Workspace(ctx, parsedID)
	if err != nil {
		return FetchResult{}, err
	}
	workspaceJSON, err := json.Marshal(workspace)
	if err != nil {
		return FetchResult{}, xerrors.Errorf("failed to marshal workspace: %w", err)
	}
	return FetchResult{
		ID:    workspace.ID.String(),
		Title: workspace.Name,
		Text:  string(workspaceJSON),
		URL:   fmt.Sprintf("%s/%s/%s", deps.ServerURL(), workspace.OwnerName, workspace.Name),
	}, nil
}

func fetchTemplate(ctx context.Context, deps Deps, templateID string) (FetchResult, error) {
	parsedID, err := uuid.Parse(templateID)
	if err != nil {
		return FetchResult{}, xerrors.Errorf("invalid template ID, must be a valid UUID: %w", err)
	}
	template, err := deps.coderClient.Template(ctx, parsedID)
	if err != nil {
		return FetchResult{}, err
	}
	templateJSON, err := json.Marshal(template)
	if err != nil {
		return FetchResult{}, xerrors.Errorf("failed to marshal template: %w", err)
	}
	return FetchResult{
		ID:    template.ID.String(),
		Title: template.DisplayName,
		Text:  string(templateJSON),
		URL:   fmt.Sprintf("%s/templates/%s/%s", deps.ServerURL(), template.OrganizationName, template.Name),
	}, nil
}

type FetchArgs struct {
	ID string `json:"id"`
}

type FetchResult struct {
	ID       string            `json:"id"`
	Title    string            `json:"title"`
	Text     string            `json:"text"`
	URL      string            `json:"url"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Implements the "fetch" tool as described in https://platform.openai.com/docs/mcp#fetch-tool.
// From my experiments with ChatGPT, it seems that it does not see the description that is
// provided in the tool definition. ChatGPT sees "fetch" as a very simple tool that can take
// an ID returned by the "search" tool and return the full details of the object.
var ChatGPTFetch = Tool[FetchArgs, FetchResult]{
	Tool: aisdk.Tool{
		Name: ToolNameChatGPTFetch,
		Description: `Fetch a template or workspace.

		ID is a unique identifier for the template or workspace. It is a combination of the type and the ID.

		# Examples

		Fetch a template with ID "56f13b5e-be0f-4a17-bdb2-aaacc3353ea7".

		` + "```" + `json
		{
			"id": "template:56f13b5e-be0f-4a17-bdb2-aaacc3353ea7"
		}
		` + "```" + `

		Fetch a workspace with ID "fcb6fc42-ba88-4175-9508-88e6a554a61a".

		` + "```" + `json
		{
			"id": "workspace:fcb6fc42-ba88-4175-9508-88e6a554a61a"
		}
		` + "```" + `
		`,

		Schema: aisdk.Schema{
			Properties: map[string]any{
				"id": map[string]any{
					"type": "string",
				},
			},
			Required: []string{"id"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args FetchArgs) (FetchResult, error) {
		objectID, err := parseObjectID(args.ID)
		if err != nil {
			return FetchResult{}, err
		}
		switch objectID.Type {
		case ObjectTypeTemplate:
			return fetchTemplate(ctx, deps, objectID.ID)
		case ObjectTypeWorkspace:
			return fetchWorkspace(ctx, deps, objectID.ID)
		}
		return FetchResult{}, xerrors.Errorf("reached unreachable code with object ID: %s", args.ID)
	},
}
