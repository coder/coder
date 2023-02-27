# Support Links

Support links let admins adjust the user dropdown menu to include links referring to internal company resources. The menu section replaces the original menu positions: documentation, report a bug to GitHub, or join the Discord server.

![support links](../images/admin/support-links.png)

Custom links can be set in the deployment configuration using the `server.yaml` file:

```yaml
support:
  links:
    - name: "On-call 🔥"
      target: "http://on-call.example.internal"
      icon: "bug"
    - name: "😉 Getting started with Go!"
      target: "https://go.dev/"
    - name: "Community"
      target: "https://github.com/coder/coder"
      icon: "chat"
```

## Icons

The link icons are optional, and limited to: `bug`, `chat`, and `docs`.

## Up next

- [Enterprise](../enterprise.md)
