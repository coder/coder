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

Specify a custom URL for your enterprise's logo to be displayed on the sign in
page and in the top left corner of the dashboard. The default is the Coder logo.

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

### Kubernetes configuration

To pass in the `supportLinks` YAML file above into your Coder Kubernetes
deployment, follow the steps below.

#### 1. Create Kubernetes Secret From File

Run the below command to create the YAML file as a Kubernetes secret in your
cluster:

```console
kubectl create secret generic coder-support-links -n <coder-namespace> --from-file=config.yaml
```

#### 2. Mount Secret as Volume in Helm Chart

Next, update your Helm chart values as follows:

```yaml
coder:
  env:
    - name: CODER_CONFIG_PATH
      value: /etc/coder/config.yaml
  volumes:
    - name: coder-config
      secret:
        secretName: coder-support-links
  volumeMounts:
    - name: coder-config
      mountPath: /etc/coder/
```

#### 3. Upgrade Coder

Lastly, upgrade Coder using the following command:

```console
helm upgrade coder coder-v2/coder -n <coder-namespace> -f <values-file.yaml>
```

## Up next

- [Enterprise](../enterprise.md)
