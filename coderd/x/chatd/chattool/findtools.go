package chattool

import (
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"charm.land/fantasy"
)

const (
	FindToolsName          = "find_tools"
	findToolsMaxMatches    = 20
	findToolsCatalogTokens = 4000
)

var findToolsTokenSeparator = regexp.MustCompile(`[^a-z0-9]+`)

// FindToolCatalogEntry is the searchable metadata for one deferred tool.
type FindToolCatalogEntry struct {
	Name              string
	Description       string
	Server            string
	ServerDescription string
	ParameterText     string
}

// FindToolsCall records one catalog search for logging and metrics.
type FindToolsCall struct {
	Queries       []string
	Names         []string
	MatchCount    int
	Activated     []string
	TotalDeferred int
}

type FindToolsOptions struct {
	Entries []FindToolCatalogEntry
	OnCall  func(context.Context, FindToolsCall)
}

type FindToolsArgs struct {
	Queries []string `json:"queries,omitempty"`
	Names   []string `json:"names,omitempty"`
}

type FindToolsMatch struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// FindToolsResult is persisted as the tool result and re-read on later steps.
type FindToolsResult struct {
	Matches       []FindToolsMatch `json:"matches"`
	Activated     []string         `json:"activated"`
	TotalDeferred int              `json:"total_deferred"`
}

// FindTools returns the built-in used to discover deferred MCP tool schemas.
func FindTools(options FindToolsOptions) fantasy.AgentTool {
	entries := slices.Clone(options.Entries)
	return fantasy.NewAgentTool(
		FindToolsName,
		buildFindToolsDescription(entries),
		func(ctx context.Context, args FindToolsArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if len(args.Queries) == 0 && len(args.Names) == 0 {
				return fantasy.NewTextErrorResponse("at least one query or name is required"), nil
			}
			result := SearchTools(entries, args)
			if options.OnCall != nil {
				options.OnCall(ctx, FindToolsCall{
					Queries:       args.Queries,
					Names:         args.Names,
					MatchCount:    len(result.Matches),
					Activated:     result.Activated,
					TotalDeferred: result.TotalDeferred,
				})
			}
			return marshalToolResponse(result), nil
		},
	)
}

// SearchTools scores entries against the query tokens, keeps the top
// matches, and always includes exact name activations.
func SearchTools(entries []FindToolCatalogEntry, args FindToolsArgs) FindToolsResult {
	byName := make(map[string]FindToolCatalogEntry, len(entries))
	for _, entry := range entries {
		byName[entry.Name] = entry
	}

	var queryTokens []string
	for _, query := range args.Queries {
		queryTokens = append(queryTokens, tokenizeFindTools(query)...)
	}

	type scoredEntry struct {
		entry FindToolCatalogEntry
		score int
	}
	scored := make([]scoredEntry, 0, len(entries))
	for _, entry := range entries {
		score := 0
		for _, token := range queryTokens {
			score += scoreFindToolToken(entry, token)
		}
		if score > 0 {
			scored = append(scored, scoredEntry{entry: entry, score: score})
		}
	}
	slices.SortFunc(scored, func(a, b scoredEntry) int {
		if a.score != b.score {
			return b.score - a.score
		}
		return strings.Compare(a.entry.Name, b.entry.Name)
	})
	if len(scored) > findToolsMaxMatches {
		scored = scored[:findToolsMaxMatches]
	}

	matches := make([]FindToolsMatch, 0, len(scored)+len(args.Names))
	activatedSet := make(map[string]struct{}, len(scored)+len(args.Names))
	for _, item := range scored {
		matches = append(matches, FindToolsMatch{Name: item.entry.Name, Description: item.entry.Description})
		activatedSet[item.entry.Name] = struct{}{}
	}
	for _, name := range args.Names {
		entry, ok := byName[name]
		if !ok {
			continue
		}
		if _, exists := activatedSet[name]; !exists {
			matches = append(matches, FindToolsMatch{Name: entry.Name, Description: entry.Description})
			activatedSet[name] = struct{}{}
		}
	}
	activated := make([]string, 0, len(activatedSet))
	for name := range activatedSet {
		activated = append(activated, name)
	}
	slices.Sort(activated)
	return FindToolsResult{Matches: matches, Activated: activated, TotalDeferred: len(entries)}
}

func tokenizeFindTools(value string) []string {
	parts := findToolsTokenSeparator.Split(strings.ToLower(value), -1)
	return slices.DeleteFunc(parts, func(part string) bool { return part == "" })
}

func scoreFindToolToken(entry FindToolCatalogEntry, token string) int {
	name := strings.ToLower(entry.Name)
	nameTokens := tokenizeFindTools(name)
	score := 0
	if slices.Contains(nameTokens, token) {
		score += 8
	} else if strings.Contains(name, token) {
		score += 5
	}
	if slices.Contains(tokenizeFindTools(entry.Description), token) {
		score += 2
	}
	if slices.Contains(tokenizeFindTools(entry.ParameterText), token) {
		score++
	}
	return score
}

func buildFindToolsDescription(entries []FindToolCatalogEntry) string {
	const usage = "Search deferred MCP tools by keyword, activate exact tool names, or scope queries with a server prefix. Calling a cataloged tool directly by name is allowed and auto-loads its schema, but search first for unfamiliar tools.\n\n"
	groups := groupFindToolsEntries(entries)
	catalog := detailedFindToolsCatalog(groups)
	if estimatedFindToolsTokens(usage+catalog) > findToolsCatalogTokens {
		catalog = namesOnlyFindToolsCatalog(groups)
	}
	if estimatedFindToolsTokens(usage+catalog) > findToolsCatalogTokens {
		catalog = countsOnlyFindToolsCatalog(groups)
	}
	return usage + catalog
}

func detailedFindToolsCatalog(groups []findToolsGroup) string {
	var b strings.Builder
	for _, group := range groups {
		writeFindToolsGroupHeader(&b, group)
		for _, entry := range group.entries {
			_, _ = b.WriteString("- ")
			_, _ = b.WriteString(entry.Name)
			_, _ = b.WriteString(" - ")
			_, _ = b.WriteString(truncateFindToolsSummary(entry.Description, 80))
			_ = b.WriteByte('\n')
		}
	}
	return b.String()
}

func namesOnlyFindToolsCatalog(groups []findToolsGroup) string {
	var b strings.Builder
	for _, group := range groups {
		writeFindToolsGroupHeader(&b, group)
		names := make([]string, 0, len(group.entries))
		for _, entry := range group.entries {
			names = append(names, entry.Name)
		}
		_, _ = b.WriteString(strings.Join(names, " "))
		_ = b.WriteByte('\n')
	}
	return b.String()
}

func countsOnlyFindToolsCatalog(groups []findToolsGroup) string {
	var b strings.Builder
	for _, group := range groups {
		_, _ = b.WriteString("## ")
		_, _ = b.WriteString(group.server)
		_, _ = b.WriteString(" (")
		_, _ = b.WriteString(strconv.Itoa(len(group.entries)))
		_, _ = b.WriteString(" tools)\n")
	}
	return b.String()
}

func writeFindToolsGroupHeader(b *strings.Builder, group findToolsGroup) {
	_, _ = b.WriteString("## ")
	_, _ = b.WriteString(group.server)
	if summary := truncateFindToolsSummary(group.description, 60); summary != "" {
		_, _ = b.WriteString(" - ")
		_, _ = b.WriteString(summary)
	}
	_ = b.WriteByte('\n')
}

type findToolsGroup struct {
	server      string
	description string
	entries     []FindToolCatalogEntry
}

func groupFindToolsEntries(entries []FindToolCatalogEntry) []findToolsGroup {
	grouped := make(map[string]*findToolsGroup)
	for _, entry := range entries {
		server := entry.Server
		if server == "" {
			server = "workspace"
		}
		group := grouped[server]
		if group == nil {
			group = &findToolsGroup{server: server, description: entry.ServerDescription}
			grouped[server] = group
		}
		group.entries = append(group.entries, entry)
	}
	groups := make([]findToolsGroup, 0, len(grouped))
	for _, group := range grouped {
		slices.SortFunc(group.entries, func(a, b FindToolCatalogEntry) int { return strings.Compare(a.Name, b.Name) })
		groups = append(groups, *group)
	}
	slices.SortFunc(groups, func(a, b findToolsGroup) int { return strings.Compare(a.server, b.server) })
	return groups
}

func truncateFindToolsSummary(value string, maxRunes int) string {
	line, _, _ := strings.Cut(value, "\n")
	sentence, _, _ := strings.Cut(line, ". ")
	value = strings.TrimSpace(sentence)
	if utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return strings.TrimSpace(string(runes[:maxRunes-3])) + "..."
}

func estimatedFindToolsTokens(value string) float64 {
	return float64(len(value)) / 2.5
}
