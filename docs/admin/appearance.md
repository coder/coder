# Appearance

## Support Links

Support links let admins adjust the user dropdown menu to include links referring to internal company resources. The menu section replaces the original menu positions: documentation, report a bug to GitHub, or join the Discord server.

![support links](../images/admin/support-links.png)

Custom links can be set in the deployment configuration using the `-c <yamlFile>`
flag to `coder server`.

```yaml
supportLinks:
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

## Service Banners (enterprise)

Service Banners let admins post important messages to all site users. Only Site Owners may set the service banner.

![service banners](../images/admin/service-banners.png)

You can access the Service Banner settings by navigating to
`Deployment > Service Banners`.

## Up next

- [Enterprise](../enterprise.md)
