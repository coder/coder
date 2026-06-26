# Authenticate to GitHub from your workspace

Now that you've finished [Launch your first workspace](./launch-workspace.md),
you can let your workspaces clone private GitHub repositories without a manual login each time.

The Quickstart template clones public repositories,
but a private repository needs authentication.
This guide uses GitHub,
and the same external-auth pattern works for other providers.
In this guide, you add the `coder_external_auth` data source,
watch the build stop when the provider is missing,
and connect a configured GitHub provider so private clones work.

> [!NOTE]
> This guide assumes your Quickstart template is open for editing.
> If it's not, refer to [Customize workspace startup](./customize-workspace-startup.md#open-the-template-for-editing).

## What you'll do

- ✅ Add the `coder_external_auth` data source to the template.
- ✅ Learn why the workspace can't start until the provider exists.
- ✅ Connect a configured GitHub provider and clone a private repository.

## Data sources for their side effect

You already met one [data source](https://developer.hashicorp.com/terraform/language/data-sources) in [Add a programming language](./add-a-language.md):
`coder_parameter` reads a choice from the workspace owner.

`coder_external_auth` is a data source you add for its side effect rather than its value.
Its presence tells Coder that a workspace requires an authenticated session with an external provider before it starts.
The `id` points at a provider configured on the Coder deployment:

```tf
data "coder_external_auth" "github" {
  id = "github"
}
```

## Step 1: Require GitHub authentication

Add the `coder_external_auth` block to `main.tf`,
then push a new version of the template:

```sh
coder templates push -d ~/coder-quickstart -y quickstart
```

## Step 2: Watch the build wait on a missing provider

Create a workspace from the new template version.
Unless your deployment already has a GitHub external-auth provider with the `id` `github`,
the workspace can't start.
Coder reports that the template requires an external authentication provider that doesn't exist:

```text
external auth provider "github" is not configured
```

The data source did its job.
It declared a requirement,
and Coder enforced it.
A data source like this depends on configuration that lives outside the template,
on the Coder deployment itself.
The template names the provider;
the deployment must provide it.

## Step 3: Connect a configured provider

Configure a GitHub external-auth provider on the Coder deployment,
with an `id` that matches the `id` in the data source.
This is a one-time deployment setting that connects a GitHub OAuth app to Coder.

1. In GitHub, go to **Settings** > **Developer settings** > **OAuth Apps** > **New OAuth App**.
   Set the **Authorization callback URL** to `https://coder.example.com/external-auth/github/callback`,
   replacing `coder.example.com` with your Coder access URL.
   The `github` segment matches the provider `id`.
2. Copy the app's client ID, then generate a client secret.
3. Set the provider's environment variables on the Coder server,
   using `github` as the `id` so it matches the template:

   ```env
   CODER_EXTERNAL_AUTH_0_ID="github"
   CODER_EXTERNAL_AUTH_0_TYPE=github
   CODER_EXTERNAL_AUTH_0_CLIENT_ID=<client-id>
   CODER_EXTERNAL_AUTH_0_CLIENT_SECRET=<client-secret>
   ```

4. Restart the Coder server so it loads the variables.

To use a different provider, or for fine-grained GitHub permissions,
refer to [External authentication](../../admin/external-auth/index.md).

With the provider in place,
create the workspace again.
The creation flow now prompts you to authenticate with GitHub.
After you authorize Coder,
the workspace starts and can clone your private GitHub repositories.

## What just happened

You added one data source and changed when a workspace is allowed to start:

- `coder_external_auth` declared that the workspace needs an authenticated GitHub session.
- Coder blocked the build until the named provider existed and you authorized it.

A `coder_parameter` reads an answer from the workspace owner.
A `coder_external_auth` data source reaches outside the template for a prerequisite the deployment provides.
Both are data sources,
and neither creates infrastructure.

## What's next?

You finished Customize workspace startup. Return to the [Quickstart overview](./index.md) to see the rest of the series.

## Learn more

- [External authentication](../../admin/external-auth/index.md) in the Coder documentation
- [Terraform data sources](https://developer.hashicorp.com/terraform/language/data-sources)
