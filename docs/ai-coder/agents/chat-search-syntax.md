# Conversation Search Syntax

The chat list endpoint accepts a `q` query parameter for filtering
conversations. All filters use `key:value` syntax. Bare search terms
are rejected; use `title:` for title filtering.

## Filters

| Key          | Values                              | Description                                                                                        |
|--------------|-------------------------------------|----------------------------------------------------------------------------------------------------|
| `title`      | substring                           | Case-insensitive substring match. Quote multi-word values.                                         |
| `archived`   | `true`, `false`                     | Filter by archived state. Default: `false`.                                                        |
| `has_unread` | `true`, `false`                     | Conversations with unread assistant messages.                                                      |
| `pr_status`  | `draft`, `open`, `merged`, `closed` | Linked pull request state. Comma-separated for OR.                                                 |
| `diff_url`   | URL                                 | Match by associated diff URL. Quote values containing colons.                                      |
| `pr`         | positive integer                    | Exact PR number match.                                                                             |
| `repo`       | substring                           | Case-insensitive substring match against git remote origin or URL. Quote values containing colons. |
| `pr_title`   | substring                           | Case-insensitive PR title substring match. Quote multi-word values.                                |

Multiple filters in one query combine with AND logic.

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
```

## Notes

- `title:`, `repo:`, and `pr_title:` use ILIKE matching. `%` and `_` act as wildcards.
- `pr_status:draft` means the PR is open **and** marked as a draft.
  `pr_status:open` means the PR is open and not a draft.
- Conversations without a linked diff status are excluded when `pr_status`, `pr`, `repo`, or `pr_title` is set. The `repo:` filter also matches chats tracking a branch with no PR.
- Unrecognized keys or bare terms return HTTP 400 with a validation error.
