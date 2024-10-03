# Licensing

Some features are only accessible with a Premium or Enterprise license. See our
[pricing page](https://coder.com/pricing) for more details. To try Premium
features, you can [request a trial](https://coder.com/trial) or
[contact sales](https://coder.com/contact).

<!-- markdown-link-check-disable -->

> If you are an existing customer, you can learn more our new Premium plan in
> the [Coder v2.16 blog post](https://coder.com/blog/release-recap-2-16-0)

<!-- markdown-link-check-enable -->

## Adding your license key

There are two ways to add a license to a Coder deployment:

<div class="tabs">

### Coder UI

First, ensure you have a license key
([request a trial](https://coder.com/trial)).

With an `Owner` account, navigate to `Deployment -> Licenses`, `Add a license`
then drag or select the license file with the `jwt` extension.

![Add License UI](../../images/add-license-ui.png)

### Coder CLI

First, ensure you have a license key
([request a trial](https://coder.com/trial)) and the
[Coder CLI](../../install/cli.md) installed.

1. Save your license key to disk and make note of the path
2. Open a terminal
3. Ensure you are logged into your Coder deployment

   `coder login <access url>`

4. Run

   `coder licenses add -f <path to your license key>`

</div>
