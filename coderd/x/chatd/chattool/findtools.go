package chattool

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"charm.land/fantasy"
)

const (
	FindToolsName          = "find_tools"
	findToolsMaxMatches    = 20
	findToolsCatalogTokens = 4000
)

var findToolsTokenSeparator = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// FindToolCatalogEntry is the searchable metadata for one deferred tool.
type FindToolCatalogEntry struct {
	Name              string
	Description       string
	Server            string
	ServerDescription string
	ParameterText     string
	// SchemaTokens is the estimated prompt weight of the tool's full
	// definition, used to cap how much one search may activate.
	SchemaTokens float64
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
	// SchemaTokenBudget caps the aggregate SchemaTokens all searches on
	// this tool instance may activate, so results never report
	// activations that the activation budget would immediately shed.
	// The budget is shared across calls because one step can execute
	// several searches concurrently. <= 0 means unbounded.
	SchemaTokenBudget float64
	// CatalogTokenBudget lowers the default catalog size cap so small
	// context windows are not consumed by the catalog itself. <= 0 or
	// values above the default keep the default.
	CatalogTokenBudget float64
	OnCall             func(context.Context, FindToolsCall)
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
	schemaTokensByName := make(map[string]float64, len(entries))
	for _, entry := range entries {
		schemaTokensByName[entry.Name] = entry.SchemaTokens
	}
	var budgetMu sync.Mutex
	remainingBudget := options.SchemaTokenBudget
	return fantasy.NewAgentTool(
		FindToolsName,
		buildFindToolsDescription(entries, options.CatalogTokenBudget),
		func(ctx context.Context, args FindToolsArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if len(args.Queries) == 0 && len(args.Names) == 0 {
				return fantasy.NewTextErrorResponse("at least one query or name is required"), nil
			}
			budgetMu.Lock()
			effectiveBudget := remainingBudget
			if options.SchemaTokenBudget > 0 && effectiveBudget <= 0 {
				// Keep SearchTools's first-match guarantee without
				// flipping an exhausted budget into "unbounded".
				effectiveBudget = math.SmallestNonzeroFloat64
			}
			result := SearchTools(entries, args, effectiveBudget)
			if options.SchemaTokenBudget > 0 {
				for _, name := range result.Activated {
					remainingBudget -= schemaTokensByName[name]
				}
			}
			budgetMu.Unlock()
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

// SearchTools includes exact name activations first, then fills the
// remaining match slots with the top-scored keyword matches. The shared
// cap and summary-length descriptions keep the persisted result small
// enough that generic tool-result truncation can never corrupt the
// activation JSON that later steps re-derive activations from. A
// positive schemaTokenBudget additionally stops admitting matches once
// their aggregate schema weight would exceed it, keeping at least one.
func SearchTools(entries []FindToolCatalogEntry, args FindToolsArgs, schemaTokenBudget float64) FindToolsResult {
	byName := make(map[string]FindToolCatalogEntry, len(entries))
	for _, entry := range entries {
		byName[entry.Name] = entry
	}

	queries := parseFindToolsQueries(entries, args.Queries)

	type scoredEntry struct {
		entry FindToolCatalogEntry
		score int
	}
	scored := make([]scoredEntry, 0, len(entries))
	for _, entry := range entries {
		score := 0
		for _, query := range queries {
			if query.server != "" && !strings.EqualFold(entry.Server, query.server) {
				continue
			}
			if query.server != "" && len(query.tokens) == 0 {
				score++
				continue
			}
			for _, token := range query.tokens {
				score += scoreFindToolToken(entry, token)
			}
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
	matches := make([]FindToolsMatch, 0, findToolsMaxMatches)
	activatedSet := make(map[string]struct{}, findToolsMaxMatches)
	usedSchemaTokens := 0.0
	appendMatch := func(entry FindToolCatalogEntry) {
		if _, exists := activatedSet[entry.Name]; exists {
			return
		}
		if len(matches) >= findToolsMaxMatches {
			return
		}
		if len(matches) > 0 && schemaTokenBudget > 0 && usedSchemaTokens+entry.SchemaTokens > schemaTokenBudget {
			return
		}
		usedSchemaTokens += entry.SchemaTokens
		matches = append(matches, FindToolsMatch{
			Name:        entry.Name,
			Description: truncateFindToolsSummary(entry.Description, 80),
		})
		activatedSet[entry.Name] = struct{}{}
	}
	for _, name := range args.Names {
		if entry, ok := byName[name]; ok {
			appendMatch(entry)
		}
	}
	for _, item := range scored {
		appendMatch(item.entry)
	}
	activated := make([]string, 0, len(activatedSet))
	for name := range activatedSet {
		activated = append(activated, name)
	}
	slices.Sort(activated)
	return FindToolsResult{Matches: matches, Activated: activated, TotalDeferred: len(entries)}
}

type scopedFindToolsQuery struct {
	server string
	tokens []string
}

// parseFindToolsQueries treats "server: terms" as a scope only when the
// prefix names a cataloged server, so queries like "error: timeout"
// still search normally.
func parseFindToolsQueries(entries []FindToolCatalogEntry, queries []string) []scopedFindToolsQuery {
	servers := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.Server != "" {
			servers[strings.ToLower(entry.Server)] = struct{}{}
		}
	}
	parsed := make([]scopedFindToolsQuery, 0, len(queries))
	for _, query := range queries {
		if prefix, rest, ok := strings.Cut(query, ":"); ok {
			server := strings.ToLower(strings.TrimSpace(prefix))
			if _, known := servers[server]; known {
				parsed = append(parsed, scopedFindToolsQuery{server: server, tokens: tokenizeFindTools(rest)})
				continue
			}
		}
		parsed = append(parsed, scopedFindToolsQuery{tokens: tokenizeFindTools(query)})
	}
	return parsed
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
	// Server metadata is shown in catalog headers, so its terms must be
	// searchable too. It applies to every tool on the server, so it
	// scores below tool-specific matches.
	if slices.Contains(tokenizeFindTools(entry.Server), token) ||
		slices.Contains(tokenizeFindTools(entry.ServerDescription), token) {
		score++
	}
	return score
}

func buildFindToolsDescription(entries []FindToolCatalogEntry, catalogTokenBudget float64) string {
	const usage = "Search deferred MCP tools by keyword, activate exact tool names, or scope a query to one server with a \"server: terms\" prefix. Calling a cataloged tool directly by name is allowed and auto-loads its schema, but search first for unfamiliar tools. At most 20 tools are returned and activated per call; call again for more.\n\n"
	budget := float64(findToolsCatalogTokens)
	if catalogTokenBudget > 0 && catalogTokenBudget < budget {
		budget = catalogTokenBudget
	}
	groups := groupFindToolsEntries(entries)
	catalog := detailedFindToolsCatalog(groups)
	if estimatedFindToolsTokens(usage+catalog) > budget {
		catalog = namesOnlyFindToolsCatalog(groups)
	}
	if estimatedFindToolsTokens(usage+catalog) > budget {
		catalog = countsOnlyFindToolsCatalog(groups)
	}
	// Server count and slug length are unbounded, so even the per-server
	// counts catalog needs a final constant-size fallback.
	if estimatedFindToolsTokens(usage+catalog) > budget {
		catalog = fmt.Sprintf("%d deferred tools across %d servers.\n", len(entries), len(groups))
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
