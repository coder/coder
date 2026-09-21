# Conversation Search Syntax

The chat list endpoint accepts a `q` query parameter for filtering
conversations. All filters use `key:value` syntax. Bare search terms
are rejected; use `title:` for title filtering or `search:` for
full-text search.

## Filters

| Key          | Values                              | Description                                                                                                           |
|--------------|-------------------------------------|-----------------------------------------------------------------------------------------------------------------------|
| `title`      | substring                           | Case-insensitive substring match. Quote multi-word values.                                                            |
| `archived`   | `true`, `false`                     | Filter by archived state. Default: `false`.                                                                           |
| `has_unread` | `true`, `false`                     | Conversations with unread assistant messages.                                                                         |
| `pr_status`  | `draft`, `open`, `merged`, `closed` | Linked pull request state. Comma-separated for OR.                                                                    |
| `diff_url`   | URL                                 | Match by associated diff URL. Quote values containing colons.                                                         |
| `pr`         | positive integer                    | Exact PR number match.                                                                                                |
| `repo`       | substring                           | Case-insensitive substring match against git remote origin or URL. Quote values containing colons.                    |
| `pr_title`   | substring                           | Case-insensitive PR title substring match. Quote multi-word values.                                                   |
| `source`     | `created_by_me`, `shared_with_me`   | Ownership scope. Default: `created_by_me`. Pass both values comma-separated to return owned and shared conversations. |
| `search`     | text                                | Full-text search across chat titles, PR titles, PR numbers, and message content. Quote multi-word values.             |

Multiple filters in one query combine with AND logic. `search:` cannot
be combined with `title:`, `pr_title:`, or `pr:`.

## Full-text search

`search:` uses token-based PostgreSQL full-text search:

- Case-insensitive whole-word matching with AND semantics. Quoted
  phrases, `OR`, and `-` negation follow
  [`websearch_to_tsquery`](https://www.postgresql.org/docs/current/textsearch-controls.html#TEXTSEARCH-PARSING-QUERIES)
  rules.
- Message content matches English word stems (`refactor` matches
  `refactoring`) and ignores English stopwords. Titles and PR titles
  do not stem.
- No fuzzy, semantic, or prefix matching.
- Whole-number values also match exact PR numbers.
- A value with no searchable words (punctuation only) returns an empty
  list; an empty value returns HTTP 400.
- Results use the standard chat list ordering (pinned first, then most
  recently updated) and pagination, without match snippets.
- Message content becomes searchable shortly after it is written
  (background indexing, usually within 10 minutes). Titles and PR
  titles are searchable immediately.

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

# Conversations shared with you, plus your own
?q=source:created_by_me,shared_with_me

# Full-text search across titles, PR titles, and messages
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
