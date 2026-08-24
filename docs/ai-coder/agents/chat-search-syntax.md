# Conversation Search Syntax

The chat list endpoint accepts a `q` query parameter for filtering
conversations. All filters use `key:value` syntax. Bare search terms
are rejected; use `title:` for title filtering or `search:` for
full-text search.

## Filters

| Key          | Values                              | Description                                                                                               |
|--------------|-------------------------------------|-----------------------------------------------------------------------------------------------------------|
| `title`      | substring                           | Case-insensitive substring match. Quote multi-word values.                                                |
| `archived`   | `true`, `false`                     | Filter by archived state. Default: `false`.                                                               |
| `has_unread` | `true`, `false`                     | Conversations with unread assistant messages.                                                             |
| `pr_status`  | `draft`, `open`, `merged`, `closed` | Linked pull request state. Comma-separated for OR.                                                        |
| `diff_url`   | URL                                 | Match by associated diff URL. Quote values containing colons.                                             |
| `pr`         | positive integer                    | Exact PR number match.                                                                                    |
| `repo`       | substring                           | Case-insensitive substring match against git remote origin or URL. Quote values containing colons.        |
| `pr_title`   | substring                           | Case-insensitive PR title substring match. Quote multi-word values.                                       |
| `search`     | text                                | Full-text search across chat titles, PR titles, PR numbers, and message content. Quote multi-word values. |

Multiple filters in one query combine with AND logic. `search:` cannot
be combined with `title:`, `pr_title:`, or `pr:`.

## Full-text search

`search:` uses token-based PostgreSQL full-text search with the
`english` text search configuration:

- Matching is case-insensitive. Every word in the value must match
  (AND semantics). Quoted phrases, `OR`, and `-` negation follow
  [`websearch_to_tsquery`](https://www.postgresql.org/docs/current/textsearch-controls.html#TEXTSEARCH-PARSING-QUERIES)
  semantics.
- English word stems match, so `refactor` matches `refactoring`. There
  is no fuzzy, semantic, or partial-word (prefix) matching.
- Common English stopwords (such as `the`, `or`, `and`) are dropped
  during tokenization. A value that tokenizes to no searchable words,
  including stopword-only or punctuation-only values, returns an empty
  list. An empty `search:` value returns HTTP 400.
- A value that is a whole number also matches chats by exact PR number.
- Results list matching conversations only; there are no match snippets
  or highlights.
- Results use the standard chat list ordering (pinned conversations
  first, then most recently updated) and pagination (default page size
  50).
- Message content is indexed by a background job and becomes searchable
  shortly after it is written, usually within 10 minutes. Chat titles
  and PR titles are searchable immediately.

The `title:`, `pr_title:`, and `repo:` filters are unaffected: they use
case-insensitive `ILIKE` substring matching, not full-text search.

## Examples

```sh
# Title substring (case-insensitive)
?q=title:deploy

# Multi-word title (URL-encode the space or use +)
?q=title:my+project

# Unread conversations
?q=has_unread:true

# Conversations with open or draft PRs
?q=pr_status:open,draft

# Filter by diff URL (quote values containing colons)
?q=diff_url:"https://github.com/coder/coder/pull/123"

# Combine filters
?q=title:refactor+has_unread:true+pr_status:merged

# Conversations linked to PR #42
?q=pr:42

# Conversations for a specific repository
?q=repo:coder/coder

# Conversations with a specific PR title
?q=pr_title:"fix auth bug"

# Full-text search across titles, PR titles, and messages
?q=search:deploy

# Multi-word full-text search (all words must match)
?q=search:"kubernetes restart"

# Combine full-text search with other filters
?q=search:refactor+pr_status:open
```

## Notes

- `title:`, `repo:`, and `pr_title:` use ILIKE matching. `%` and `_` act as wildcards.
- `pr_status:draft` means the PR is open **and** marked as a draft.
  `pr_status:open` means the PR is open and not a draft.
- Conversations without a linked diff status are excluded when `pr_status`, `pr`, `repo`, or `pr_title` is set. The `repo:` filter also matches chats tracking a branch with no PR.
- Unrecognized keys, bare terms, or combining `search:` with `title:`, `pr_title:`, or `pr:` return HTTP 400 with a validation error.
