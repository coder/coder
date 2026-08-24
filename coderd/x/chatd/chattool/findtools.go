package chattool

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"charm.land/fantasy"
)

const (
	FindToolsName = "find_tools"
	// Keep broad query results concise while allowing higher explicit limits.
	findToolsDefaultMatches = 10
	findToolsMaxMatches     = 20
	findToolsCatalogTokens  = 4000
	// findToolsMaxQueries and findToolsMaxQueryTokens bound scoring
	// work: queries are model output, so one call could otherwise
	// carry arbitrarily many tokens scored against every entry.
	findToolsMaxQueries     = 10
	findToolsMaxQueryTokens = 16
	// findToolsMaxNames bounds exact-name lookups the same way. Twice
	// the match cap leaves room for unknown or duplicate names.
	findToolsMaxNames = 2 * findToolsMaxMatches
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
	Queries []string `json:"queries,omitempty" description:"Task or capability keywords, matched against tool names, descriptions, parameters, and server metadata. Prefer a few specific keywords over sentences."`
	Names   []string `json:"names,omitempty" description:"Exact cataloged tool names to activate directly."`
	Limit   int      `json:"limit,omitempty" description:"Cap on total tools returned and activated per call (default 10, max 20). Exact names are always included and may exceed it."`
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
	settleStepResults func(names []string, errored []bool)
	onDecodeRejected  func(ctx context.Context)
}

func (findToolsTool) SerialToolCalls() bool { return true }

func (t findToolsTool) ObserveStepToolCalls(names []string) { t.reserveStepCalls(names) }

func (t findToolsTool) ObserveStepToolResults(names []string, errored []bool) {
	t.settleStepResults(names, errored)
}

// Run counts calls the typed wrapper rejects during argument decoding,
// which never reach the handler and would otherwise be missing from
// call metrics. The response itself still comes from the wrapper's own
// decode so its wording stays canonical.
func (t findToolsTool) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	var args FindToolsArgs
	if err := json.Unmarshal([]byte(call.Input), &args); err != nil && t.onDecodeRejected != nil {
		t.onDecodeRejected(ctx)
	}
	return t.AgentTool.Run(ctx, call)
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
	// derivation before any search activations, per call in call order,
	// while their cumulative weight fits the budget (the first always
	// fits, mirroring derivation's newest-keep rule). Only that
	// retained prefix is free to activate; a call past it is
	// unclaimable this step because derivation marks it seen at its
	// rejected position, so no same-step search can inline its schema
	// either. Errored calls (including calls rejected before execution)
	// are skipped per call when siblings settle, exactly as derivation
	// postpones them by call ID, so one tool called several times with
	// mixed outcomes admits at its first successful call's position.
	type stepToolCall struct {
		name    string
		errored bool
	}
	var stepCalls []stepToolCall
	reserved := make(map[string]struct{})
	unclaimable := make(map[string]struct{})
	// Derivation deduplicates activations by name, so a name an earlier
	// search already claimed is free for later searches in the step.
	claimedBySearch := make(map[string]struct{})
	searchClaimed := 0.0
	recompute := func() {
		clear(reserved)
		clear(unclaimable)
		charged := 0.0
		seen := make(map[string]struct{}, len(stepCalls))
		for _, call := range stepCalls {
			if call.errored {
				continue
			}
			if _, dup := seen[call.name]; dup {
				continue
			}
			seen[call.name] = struct{}{}
			weight := schemaTokensByName[call.name]
			if len(reserved) > 0 && charged+weight > options.SchemaTokenBudget {
				unclaimable[call.name] = struct{}{}
				continue
			}
			reserved[call.name] = struct{}{}
			charged += weight
		}
		remainingBudget = options.SchemaTokenBudget - charged - searchClaimed
	}
	rebuild := func(names []string, errored []bool) {
		stepCalls = stepCalls[:0]
		for i, name := range names {
			if _, ok := schemaTokensByName[name]; !ok {
				continue
			}
			stepCalls = append(stepCalls, stepToolCall{name: name, errored: len(errored) > i && errored[i]})
		}
		recompute()
	}
	reserve := func(names []string) {
		if options.SchemaTokenBudget <= 0 {
			return
		}
		budgetMu.Lock()
		defer budgetMu.Unlock()
		// Outcomes are unknown before execution, so every call charges;
		// settle rebuilds with real per-call outcomes before searches
		// run.
		rebuild(names, nil)
	}
	settle := func(names []string, errored []bool) {
		if options.SchemaTokenBudget <= 0 {
			return
		}
		budgetMu.Lock()
		defer budgetMu.Unlock()
		rebuild(names, errored)
	}
	onDecodeRejected := func(ctx context.Context) {
		if options.OnCall != nil {
			options.OnCall(ctx, FindToolsCall{
				TotalDeferred: len(entries),
				Rejection:     findToolsRejectionArguments,
			})
		}
	}
	return findToolsTool{reserveStepCalls: reserve, settleStepResults: settle, onDecodeRejected: onDecodeRejected, AgentTool: fantasy.NewAgentTool(
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
			if len(reserved) > 0 || len(unclaimable) > 0 || len(claimedBySearch) > 0 {
				searchEntries = make([]FindToolCatalogEntry, 0, len(entries))
				for _, entry := range entries {
					if _, ok := unclaimable[entry.Name]; ok {
						continue
					}
					if _, ok := reserved[entry.Name]; ok {
						entry.SchemaTokens = 0
					}
					if _, ok := claimedBySearch[entry.Name]; ok {
						entry.SchemaTokens = 0
					}
					searchEntries = append(searchEntries, entry)
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
					if _, ok := claimedBySearch[name]; ok {
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
				searchClaimed += admitted
				remainingBudget -= admitted
				for _, name := range result.Activated {
					if _, ok := reserved[name]; !ok {
						claimedBySearch[name] = struct{}{}
					}
				}
			}
			budgetMu.Unlock()
			// Unclaimable entries stay deferred; report the full count.
			result.TotalDeferred = len(entries)
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

// SearchTools prioritizes exact names, then keyword matches ranked by
// distinct query terms matched before raw score. Exact names bypass the
// per-call limit, but a hard cap keeps the persisted result safe from
// generic tool-result truncation; the second return counts
// budget-skipped matches.
func SearchTools(entries []FindToolCatalogEntry, args FindToolsArgs, budget SearchBudget) (FindToolsResult, int) {
	byName := make(map[string]FindToolCatalogEntry, len(entries))
	for _, entry := range entries {
		byName[entry.Name] = entry
	}

	queryArgs := args.Queries
	if len(queryArgs) > findToolsMaxQueries {
		queryArgs = queryArgs[:findToolsMaxQueries]
	}
	queries := parseFindToolsQueries(entries, queryArgs)

	type scoredEntry struct {
		entry    FindToolCatalogEntry
		coverage int
		score    int
	}
	scored := make([]scoredEntry, 0, len(entries))
	for _, entry := range entries {
		tokens := tokenizeFindToolsEntry(entry)
		score := 0
		matched := make(map[string]struct{})
		for _, query := range queries {
			if query.server != "" {
				if query.exact && entry.Server != query.server {
					continue
				}
				if !query.exact && !strings.EqualFold(entry.Server, query.server) {
					continue
				}
			}
			if query.server != "" && len(query.tokens) == 0 {
				score++
				// A scope-only hit counts as one covered term; ":" cannot occur in tokens.
				matched[":"+query.server] = struct{}{}
				continue
			}
			for _, token := range query.tokens {
				if tokenScore := tokens.score(token); tokenScore > 0 {
					score += tokenScore
					matched[token] = struct{}{}
				}
			}
		}
		if score > 0 {
			scored = append(scored, scoredEntry{entry: entry, coverage: len(matched), score: score})
		}
	}
	slices.SortFunc(scored, func(a, b scoredEntry) int {
		if a.coverage != b.coverage {
			return b.coverage - a.coverage
		}
		if a.score != b.score {
			return b.score - a.score
		}
		return strings.Compare(a.entry.Name, b.entry.Name)
	})
	matchLimit := args.Limit
	if matchLimit <= 0 {
		matchLimit = findToolsDefaultMatches
	}
	if matchLimit > findToolsMaxMatches {
		matchLimit = findToolsMaxMatches
	}
	matches := make([]FindToolsMatch, 0, findToolsMaxMatches)
	activatedSet := make(map[string]struct{}, findToolsMaxMatches)
	usedSchemaTokens := 0.0
	budgetSkipped := 0
	appendMatch := func(entry FindToolCatalogEntry, limit int) {
		if _, exists := activatedSet[entry.Name]; exists {
			return
		}
		if len(matches) >= limit {
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
	nameArgs := args.Names
	if len(nameArgs) > findToolsMaxNames {
		nameArgs = nameArgs[:findToolsMaxNames]
	}
	for _, name := range nameArgs {
		if entry, ok := byName[name]; ok {
			appendMatch(entry, findToolsMaxMatches)
		}
	}
	for _, item := range scored {
		appendMatch(item.entry, matchLimit)
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
	// exact scopes to the one server whose name matched byte-for-byte;
	// otherwise the scope folds case and may span case-colliding
	// servers.
	exact  bool
	tokens []string
}

// parseFindToolsQueries treats "server: terms" as a scope only when the
// prefix names a cataloged server, so queries like "error: timeout"
// still search normally. Prefixes are matched against full cataloged
// server names, longest first, because workspace server names may
// themselves contain ":". An exact-case prefix wins before the
// case-insensitive fallback, so servers whose names differ only by
// case each stay reachable by their advertised catalog name.
func parseFindToolsQueries(entries []FindToolCatalogEntry, queries []string) []scopedFindToolsQuery {
	servers := make([]string, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.Server == "" {
			continue
		}
		if _, dup := seen[entry.Server]; dup {
			continue
		}
		seen[entry.Server] = struct{}{}
		servers = append(servers, entry.Server)
	}
	// Longest first, so a server named "jira:prod" wins over "jira"
	// when both are cataloged.
	slices.SortFunc(servers, func(a, b string) int { return len(b) - len(a) })
	parsed := make([]scopedFindToolsQuery, 0, len(queries))
	for _, query := range queries {
		scoped := false
		trimmed := strings.TrimSpace(query)
		// The raw query is matched before whitespace normalization so
		// a whitespace-padded server name retained by collision
		// handling stays selectable; then the trimmed exact and
		// case-insensitive passes run as fallbacks.
		passes := []struct {
			text  string
			exact bool
		}{
			{text: query, exact: true},
			{text: trimmed, exact: true},
			{text: trimmed, exact: false},
		}
		for _, pass := range passes {
			if scoped {
				break
			}
			for _, server := range servers {
				var rest string
				if pass.exact {
					var ok bool
					rest, ok = strings.CutPrefix(pass.text, server)
					if !ok {
						continue
					}
				} else {
					var ok bool
					rest, ok = cutPrefixFold(pass.text, server)
					if !ok {
						continue
					}
				}
				rest, ok := strings.CutPrefix(strings.TrimLeft(rest, " "), ":")
				if !ok {
					continue
				}
				parsed = append(parsed, scopedFindToolsQuery{server: server, exact: pass.exact, tokens: tokenizeFindToolsQuery(rest)})
				scoped = true
				break
			}
		}
		if !scoped {
			parsed = append(parsed, autoScopeFindToolsQuery(servers, query))
		}
	}
	return parsed
}

// autoScopeFindToolsQuery scopes an unprefixed query whose words name
// exactly one cataloged server (repeats merge), so "linear issues"
// ranks like "linear: issues" rather than the server name inflating
// every tool on it. Words naming distinct servers stay unscoped as
// ambiguous.
func autoScopeFindToolsQuery(servers []string, query string) scopedFindToolsQuery {
	words := strings.Fields(query)
	// Bound model-generated words before scanning every server. Unscoped
	// fallback tokenization applies its own cap.
	if len(words) > findToolsMaxQueryTokens {
		words = words[:findToolsMaxQueryTokens]
	}
	scopeServer := ""
	scopeExact := false
	rest := make([]string, 0, len(words))
	for _, word := range words {
		server, exact, ok := matchFindToolsServerWord(servers, word)
		if !ok {
			rest = append(rest, word)
			continue
		}
		if scopeServer != "" && server != scopeServer {
			return scopedFindToolsQuery{tokens: tokenizeFindToolsQuery(query)}
		}
		scopeServer = server
		scopeExact = scopeExact || exact
	}
	if scopeServer == "" {
		return scopedFindToolsQuery{tokens: tokenizeFindToolsQuery(query)}
	}
	return scopedFindToolsQuery{server: scopeServer, exact: scopeExact, tokens: tokenizeFindToolsQuery(strings.Join(rest, " "))}
}

// matchFindToolsServerWord resolves one query word to a cataloged
// server name, exact-case first. Folded matches report exact=false so
// scoring spans case-colliding servers, like a folded prefix scope.
func matchFindToolsServerWord(servers []string, word string) (server string, exact bool, ok bool) {
	folded := ""
	for _, candidate := range servers {
		if word == candidate {
			return candidate, true, true
		}
		if folded == "" && strings.EqualFold(word, candidate) {
			folded = candidate
		}
	}
	if folded != "" {
		return folded, false, true
	}
	return "", false, false
}

// cutPrefixFold is a case-insensitive strings.CutPrefix. It compares
// rune by rune with the same simple folding as strings.EqualFold, so a
// prefix whose folded form differs in UTF-8 byte length (like S and
// the long s) still matches and the cut lands on a rune boundary,
// which byte-length slicing cannot guarantee.
func cutPrefixFold(s, prefix string) (string, bool) {
	rest := s
	for _, prefixRune := range prefix {
		restRune, size := utf8.DecodeRuneInString(rest)
		if size == 0 || !runesFoldEqual(restRune, prefixRune) {
			return "", false
		}
		rest = rest[size:]
	}
	return rest, true
}

func runesFoldEqual(a, b rune) bool {
	if a == b {
		return true
	}
	for r := unicode.SimpleFold(a); r != a; r = unicode.SimpleFold(r) {
		if r == b {
			return true
		}
	}
	return false
}

func tokenizeFindTools(value string) []string {
	parts := findToolsTokenSeparator.Split(strings.ToLower(value), -1)
	return slices.DeleteFunc(parts, func(part string) bool { return part == "" })
}

// tokenizeFindToolsQuery caps model-supplied query tokens; catalog
// fields are tokenized uncapped so every term stays searchable.
func tokenizeFindToolsQuery(value string) []string {
	tokens := tokenizeFindTools(value)
	if len(tokens) > findToolsMaxQueryTokens {
		tokens = tokens[:findToolsMaxQueryTokens]
	}
	return tokens
}

// findToolsEntryTokens holds an entry's fields tokenized once per
// search, so scoring a token is a set lookup instead of re-splitting
// name, description, parameter, and server text for every query token.
type findToolsEntryTokens struct {
	name        string
	nameTokens  map[string]struct{}
	description map[string]struct{}
	parameters  map[string]struct{}
	server      map[string]struct{}
}

func tokenizeFindToolsEntry(entry FindToolCatalogEntry) findToolsEntryTokens {
	toSet := func(value string) map[string]struct{} {
		tokens := tokenizeFindTools(value)
		set := make(map[string]struct{}, len(tokens))
		for _, token := range tokens {
			set[token] = struct{}{}
		}
		return set
	}
	return findToolsEntryTokens{
		name:        strings.ToLower(entry.Name),
		nameTokens:  toSet(entry.Name),
		description: toSet(entry.Description),
		parameters:  toSet(entry.ParameterText),
		// Server metadata is shown in catalog headers, so its terms
		// must be searchable too. It applies to every tool on the
		// server, so it scores below tool-specific matches.
		server: toSet(entry.Server + " " + entry.ServerDescription),
	}
}

func (t findToolsEntryTokens) score(token string) int {
	score := 0
	if _, ok := t.nameTokens[token]; ok {
		score += 8
	} else if strings.Contains(t.name, token) {
		score += 5
	}
	if _, ok := t.description[token]; ok {
		score += 2
	}
	if _, ok := t.parameters[token]; ok {
		score++
	}
	if _, ok := t.server[token]; ok {
		score++
	}
	return score
}

func buildFindToolsDescription(entries []FindToolCatalogEntry, catalogTokenBudget float64) string {
	const usage = "The MCP tools cataloged below are deferred: not in your tool list until activated. Search by keyword, activate exact tool names, or scope with a \"server: terms\" prefix; matches activate and become callable on the next step. Direct calls to cataloged tools also work, but search first for unfamiliar tools. limit caps total results per call (default 10, max 20); exact names bypass it but still spend the shared schema budget. Narrow the query or raise limit for more.\n\n"
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
		// Callers assign every entry a non-empty Server so display,
		// scope matching, and scoring share one identity; an empty
		// value groups as-is rather than under a label scopes cannot
		// reach.
		server := entry.Server
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
