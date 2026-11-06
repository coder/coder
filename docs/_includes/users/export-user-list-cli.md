Use `users list` with `jq` to export the list of users to a CSV file:

```sh
coder users list --output json | \
  jq -r '["username","email","created_at","status"], (.[] | [.username, .email, .created_at, .status]) | @csv' > users.csv
```

Visit the [users list](../../reference/cli/users/list.md) documentation for more options.
