# Appearance (enterprise)

Customize the look of your Coder deployment to meet your enterprise
requirements.

You can access the Appearance settings by navigating to
`Deployment > Appearance`.

![application name and logo url](../images/admin/application-name-logo-url.png)

## Application Name

Specify a custom application name to be displayed on the login page. The default
is Coder.

## Logo URL

Specify a custom URL for your enterprise's logo to be displayed in the top left
corner of the dashboard. The default is the Coder logo.

## Service Banner

![service banner](../images/admin/service-banner-config.png)

A Service Banner lets admins post important messages to all site users. Only
Site Owners may set the service banner.

Example: Notify users of scheduled maintenance of the Coder deployment.

![service banner maintenance](../images/admin/service-banner-maintenance.png)

Example: Adhere to government network classification requirements and notify
users of which network their Coder deployment is on.

![service banner secret](../images/admin/service-banner-secret.png)

## OIDC Login Button Customization

[Use environment variables to customize](../auth#oidc-login-customization) the
text and icon on the OIDC button on the Sign In page.

## Support Links

Support links let admins adjust the user dropdown menu to include links
referring to internal company resources. The menu section replaces the original
menu positions: documentation, report a bug to GitHub, or join the Discord
server.

![support links](../images/admin/support-links.png)

Custom links can be set in the deployment configuration using the
`-c <yamlFile>` flag to `coder server`.

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

### Icons

The link icons are optional, and limited to: `bug`, `chat`, and `docs`.

## Up next

- [Enterprise](../enterprise.md)
