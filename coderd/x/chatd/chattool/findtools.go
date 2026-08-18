package chattool

import (
	"context"
	"fmt"
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
	// findToolsSpentBudgetFloor replaces a spent or over-reserved budget
	// for searches so zero-cost reserved names remain activatable while
	// any real schema weight still exceeds it.
	findToolsSpentBudgetFloor = 0.000001
)

var findToolsTokenSeparator = regexp.MustCompile(`[^\p{L}\p{N}]+`)

const findToolsBudgetExhausted = "the schema activation budget for this conversation is exhausted; call a cataloged tool directly by name to activate it in place of the least recently used schema"

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
	// Rejection is empty for successful searches. Rejected calls carry
	// "budget" or "arguments" so callers can count them without
	// polluting match or activation statistics.
	Rejection string
}

const (
	findToolsRejectionBudget    = "budget"
	findToolsRejectionArguments = "arguments"
)

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

// findToolsTool opts find_tools into in-order execution when one step
// contains several calls, so the shared schema budget is claimed in
// tool-call order rather than scheduler order. It also observes the
// step's sibling tool-call names so direct calls to deferred tools are
// charged against the budget before any search admits activations.
type findToolsTool struct {
	fantasy.AgentTool
	reserveStepCalls  func(names []string)
	settleStepResults func(succeeded, errored []string)
}

func (findToolsTool) SerialToolCalls() bool { return true }

func (t findToolsTool) ObserveStepToolCalls(names []string) { t.reserveStepCalls(names) }

func (t findToolsTool) ObserveStepToolResults(succeeded, errored []string) {
	t.settleStepResults(succeeded, errored)
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
	// Direct calls to deferred tools in the same step are admitted by
	// derivation before any search activations, so their schema weight
	// is reserved out of the budget before searches run. Reserved names
	// stay free to activate because derivation already retains them.
	reserved := make(map[string]struct{})
	executedOK := make(map[string]struct{})
	reserve := func(names []string) {
		if options.SchemaTokenBudget <= 0 {
			return
		}
		budgetMu.Lock()
		defer budgetMu.Unlock()
		for _, name := range names {
			weight, ok := schemaTokensByName[name]
			if !ok {
				continue
			}
			if _, dup := reserved[name]; dup {
				continue
			}
			reserved[name] = struct{}{}
			remainingBudget -= weight
		}
	}
	// Derivation admits errored direct calls only with leftover budget,
	// so once a step ends their pre-execution reservation is refunded
	// and later searches regain the weight. Names that ever executed
	// successfully keep their reservation: derivation admits them at
	// full priority. Denied calls never produce step results here and
	// stay charged, which only under-claims.
	settle := func(succeeded, errored []string) {
		if options.SchemaTokenBudget <= 0 {
			return
		}
		budgetMu.Lock()
		defer budgetMu.Unlock()
		for _, name := range succeeded {
			if _, ok := schemaTokensByName[name]; ok {
				executedOK[name] = struct{}{}
			}
		}
		for _, name := range errored {
			if _, ok := executedOK[name]; ok {
				continue
			}
			if _, ok := reserved[name]; !ok {
				continue
			}
			delete(reserved, name)
			remainingBudget += schemaTokensByName[name]
		}
	}
	return findToolsTool{reserveStepCalls: reserve, settleStepResults: settle, AgentTool: fantasy.NewAgentTool(
		FindToolsName,
		buildFindToolsDescription(entries, options.CatalogTokenBudget),
		func(ctx context.Context, args FindToolsArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if len(args.Queries) == 0 && len(args.Names) == 0 {
				if options.OnCall != nil {
					options.OnCall(ctx, FindToolsCall{
						TotalDeferred: len(entries),
						Rejection:     findToolsRejectionArguments,
					})
				}
				return fantasy.NewTextErrorResponse("at least one query or name is required"), nil
			}
			budgetMu.Lock()
			searchEntries := entries
			if len(reserved) > 0 {
				searchEntries = slices.Clone(entries)
				for i := range searchEntries {
					if _, ok := reserved[searchEntries[i].Name]; ok {
						searchEntries[i].SchemaTokens = 0
					}
				}
			}
			searchBudget := remainingBudget
			if options.SchemaTokenBudget > 0 && searchBudget <= 0 {
				// A spent budget still admits zero-cost reserved names,
				// so search with a floor instead of failing outright.
				searchBudget = findToolsSpentBudgetFloor
			}
			budgetTouched := options.SchemaTokenBudget > 0 && remainingBudget < options.SchemaTokenBudget
			result, budgetSkipped := SearchTools(searchEntries, args, SearchBudget{
				SchemaTokens:         searchBudget,
				AllowFirstOverBudget: !budgetTouched,
			})
			if options.SchemaTokenBudget > 0 {
				if len(result.Activated) == 0 && budgetSkipped > 0 {
					budgetMu.Unlock()
					if options.OnCall != nil {
						options.OnCall(ctx, FindToolsCall{
							Queries:       args.Queries,
							Names:         args.Names,
							TotalDeferred: len(entries),
							Rejection:     findToolsRejectionBudget,
						})
					}
					return fantasy.NewTextErrorResponse(findToolsBudgetExhausted), nil
				}
				admitted := 0.0
				for _, name := range result.Activated {
					if _, ok := reserved[name]; ok {
						continue
					}
					admitted += schemaTokensByName[name]
				}
				// Defensive invariant: with allowFirstOverBudget off,
				// a touched budget can never admit an over-claim. If
				// bookkeeping ever drifts, fail loudly rather than
				// report activations derivation would shed.
				if admitted > 0 && admitted > remainingBudget && budgetTouched {
					budgetMu.Unlock()
					if options.OnCall != nil {
						options.OnCall(ctx, FindToolsCall{
							Queries:       args.Queries,
							Names:         args.Names,
							TotalDeferred: len(entries),
							Rejection:     findToolsRejectionBudget,
						})
					}
					return fantasy.NewTextErrorResponse(findToolsBudgetExhausted), nil
				}
				remainingBudget -= admitted
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
	)}
}

// SearchBudget bounds the schema weight one search may activate.
type SearchBudget struct {
	// SchemaTokens is the remaining activation budget. <= 0 means
	// unbounded.
	SchemaTokens float64
	// AllowFirstOverBudget admits the first match even over budget.
	// Callers set it only while the shared budget is untouched, where
	// derivation's newest-keep rule retains a sole over-budget claim.
	AllowFirstOverBudget bool
}

// SearchTools includes exact name activations first, then fills the
// remaining match slots with the top-scored keyword matches. The shared
// cap and summary-length descriptions keep the persisted result small
// enough that generic tool-result truncation can never corrupt the
// activation JSON that later steps re-derive activations from. A
// positive budget additionally skips matches whose schema weight would
// push the aggregate past it, admitting later matches that still fit.
// The second result counts matches skipped for budget, so callers can
// tell an exhausted budget from no matches.
func SearchTools(entries []FindToolCatalogEntry, args FindToolsArgs, budget SearchBudget) (FindToolsResult, int) {
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
	budgetSkipped := 0
	appendMatch := func(entry FindToolCatalogEntry) {
		if _, exists := activatedSet[entry.Name]; exists {
			return
		}
		if len(matches) >= findToolsMaxMatches {
			return
		}
		overBudget := budget.SchemaTokens > 0 && usedSchemaTokens+entry.SchemaTokens > budget.SchemaTokens
		if overBudget && (len(matches) > 0 || !budget.AllowFirstOverBudget) {
			budgetSkipped++
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
	return FindToolsResult{Matches: matches, Activated: activated, TotalDeferred: len(entries)}, budgetSkipped
}

type scopedFindToolsQuery struct {
	server string
	tokens []string
}

// parseFindToolsQueries treats "server: terms" as a scope only when the
// prefix names a cataloged server, so queries like "error: timeout"
// still search normally. Prefixes are matched against full cataloged
// server names, longest first, because workspace server names may
// themselves contain ":".
func parseFindToolsQueries(entries []FindToolCatalogEntry, queries []string) []scopedFindToolsQuery {
	servers := make([]string, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		server := strings.ToLower(entry.Server)
		if server == "" {
			continue
		}
		if _, dup := seen[server]; dup {
			continue
		}
		seen[server] = struct{}{}
		servers = append(servers, server)
	}
	// Longest first, so a server named "jira:prod" wins over "jira"
	// when both are cataloged.
	slices.SortFunc(servers, func(a, b string) int { return len(b) - len(a) })
	parsed := make([]scopedFindToolsQuery, 0, len(queries))
	for _, query := range queries {
		scoped := false
		trimmed := strings.ToLower(strings.TrimSpace(query))
		for _, server := range servers {
			rest, ok := strings.CutPrefix(trimmed, server)
			if !ok {
				continue
			}
			rest, ok = strings.CutPrefix(strings.TrimLeft(rest, " "), ":")
			if !ok {
				continue
			}
			parsed = append(parsed, scopedFindToolsQuery{server: server, tokens: tokenizeFindTools(rest)})
			scoped = true
			break
		}
		if !scoped {
			parsed = append(parsed, scopedFindToolsQuery{tokens: tokenizeFindTools(query)})
		}
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
