package toolsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/xerrors"

	"github.com/google/uuid"

	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/codersdk"
)

func getServerURL(deps Deps) string {
	serverURLCopy := *deps.coderClient.URL
	serverURLCopy.Path = ""
	serverURLCopy.RawQuery = ""
	return serverURLCopy.String()
}

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

func searchTemplates(ctx context.Context, deps Deps) ([]SearchResultItem, error) {
	serverURL := getServerURL(deps)
	templates, err := deps.coderClient.Templates(ctx, codersdk.TemplateFilter{})
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

func searchWorkspaces(ctx context.Context, deps Deps, owner string) ([]SearchResultItem, error) {
	serverURL := getServerURL(deps)
	if owner == "" {
		owner = "me"
	}
	workspaces, err := deps.coderClient.Workspaces(ctx, codersdk.WorkspaceFilter{
		Owner: owner,
	})
	if err != nil {
		return nil, err
	}
	results := make([]SearchResultItem, len(workspaces.Workspaces))
	for i, workspace := range workspaces.Workspaces {
		results[i] = SearchResultItem{
			ID:    createObjectID(ObjectTypeWorkspace, workspace.ID.String()).String(),
			Title: workspace.Name,
			Text:  fmt.Sprintf("Owner: %s\nTemplate: %s\nLatest transition: %s", owner, workspace.TemplateDisplayName, workspace.LatestBuild.Transition),
			URL:   fmt.Sprintf("%s/%s/%s", serverURL, owner, workspace.Name),
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
	Type           SearchQueryType
	WorkspaceOwner string
}

func parseSearchQuery(query string) (SearchQuery, error) {
	parts := strings.Split(query, ":")
	switch SearchQueryType(parts[0]) {
	case SearchQueryTypeTemplates:
		// expected format: templates
		return SearchQuery{
			Type: SearchQueryTypeTemplates,
		}, nil
	case SearchQueryTypeWorkspaces:
		// expected format: workspaces:owner
		owner := "me"
		if len(parts) == 2 {
			owner = parts[1]
		} else if len(parts) != 1 {
			return SearchQuery{}, xerrors.Errorf("invalid query: %s", query)
		}
		return SearchQuery{
			Type:           SearchQueryTypeWorkspaces,
			WorkspaceOwner: owner,
		}, nil
	}
	return SearchQuery{}, xerrors.Errorf("invalid query: %s", query)
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
		Description: `Search for templates, workspaces, and files in workspaces.

To pick what you want to search for, use the following query formats:

- ` + "`" + `templates` + "`" + `: List all templates. This query is not parameterized.
- ` + "`" + `workspaces:$owner` + "`" + `: List workspaces belonging to a user. If owner is not specified, the current user is used. The special value ` + "`" + `me` + "`" + ` can be used to search for workspaces owned by the current user.

# Examples

## Listing templates

List all templates.

` + "```" + `json
{
	"query": "templates"
}
` + "```" + `

## Listing workspaces

List all workspaces belonging to the current user.

` + "```" + `json
{
	"query": "workspaces:me"
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
	"query": "workspaces:josh"
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
			results, err := searchTemplates(ctx, deps)
			if err != nil {
				return SearchResult{}, err
			}
			return SearchResult{Results: results}, nil
		case SearchQueryTypeWorkspaces:
			results, err := searchWorkspaces(ctx, deps, query.WorkspaceOwner)
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
		URL:   fmt.Sprintf("%s/%s/%s", getServerURL(deps), workspace.OwnerName, workspace.Name),
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
		URL:   fmt.Sprintf("%s/templates/%s/%s", getServerURL(deps), template.OrganizationName, template.Name),
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
