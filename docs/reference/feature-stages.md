# Feature stages

Some Coder features are released in feature stages before they are generally
available.

If you encounter an issue with any Coder feature, please submit a
[GitHub issue](https://github.com/coder/coder/issues) or join the
[Coder Discord](https://discord.gg/coder).

## Feature stages

| Feature stage                          | Stable | Production-ready | Support               | Description                                                                                                                   |
|----------------------------------------|--------|------------------|-----------------------|-------------------------------------------------------------------------------------------------------------------------------|
| [Early Access](#early-access-features) | No     | No               | GitHub issues         | For staging only. Not feature-complete or stable. Disabled by default.                                                        |
| [Beta](#beta)                          | No     | Not fully        | Docs, Discord, GitHub | Publicly available. In active development with minor bugs. Suitable for staging; optional for production. Not covered by SLA. |
| [GA](#general-availability-ga)         | Yes    | Yes              | License-based         | Stable and tested. Enabled by default. Fully documented. Support based on license.                                            |

## Early access features

- **Stable**: No
- **Production-ready**: No
- **Support**: GitHub issues

Early access features are neither feature-complete nor stable. We do not
recommend using early access features in production deployments.

Coder sometimes releases early access features that are available for use, but
are disabled by default. You shouldn't use early access features in production
because they might cause performance or stability issues. Early access features
can be mostly feature-complete, but require further internal testing and remain
in the early access stage for at least one month.

Coder may make significant changes or revert features to a feature flag at any
time.

If you plan to activate an early access feature, we suggest that you use a
staging deployment.

<details><summary>To enable early access features:</summary>

Use the [Coder CLI](../install/cli.md) `--experiments` flag to enable early
access features:

- Enable all early access features:

  ```sh
  coder server --experiments=*
  ```

- Enable multiple early access features:

  ```sh
  coder server --experiments=feature1,feature2
  ```

You can also use the `CODER_EXPERIMENTS`
[environment variable](../admin/setup/index.md).

You can opt-out of a feature after you've enabled it.

</details>

### Target experiments at runtime

Some early access features accept runtime rules.
A rule turns the experiment on or off for every user, or targets it to some users, without a restart.
Only user-scoped experiments accept rules.
Other experiments, such as `no_nats_pubsub` or `workspace-capable-licensing`, read only the startup `--experiments` list and need a restart to change.

> [!NOTE]
> The rules API and the hidden `coder exp experiment-rules` command are experimental.
> They can change or be removed without notice.
> There is no UI editor yet.

You need the Owner role to change a rule.
The Auditor role can list rules.

Run `coder exp experiment-rules list` to show each user-scoped experiment, its startup default, and its rule.
The list also shows stored rules that target other experiments as `ignored`, because they have no effect.

#### Rule modes

Each user-scoped experiment has at most one rule:

| Rule                  | Effect                                                                     | Command                                                   |
|-----------------------|----------------------------------------------------------------------------|-----------------------------------------------------------|
| No rule, or `inherit` | Uses the startup `--experiments` default                                   | `coder exp experiment-rules reset <experiment>`           |
| `on`                  | On for every user                                                          | `coder exp experiment-rules on <experiment>`              |
| `off`                 | Off for every user, even if the experiment is enabled at startup           | `coder exp experiment-rules off <experiment>`             |
| `condition`           | On for users whose [CEL](https://cel.dev) condition is true, off otherwise | `coder exp experiment-rules set <experiment> <condition>` |

For example, to turn on an experiment for users in one group, widen it to every member of the `coder` organization, and then turn it off for everyone:

```sh
coder exp experiment-rules set mcp-tool-search '"coder/beta-testers" in user.groups'
coder exp experiment-rules set mcp-tool-search '"coder/Everyone" in user.groups'
coder exp experiment-rules off mcp-tool-search
```

Reset doesn't turn an experiment off.
It restores the startup default, which can enable the experiment for every user.
The command prints a warning when that happens.
To turn an experiment off for everyone, use `off`.

Each change increases the rule's revision.
Changing commands accept `--expected-revision` and fail when the stored rule has a different revision.
Without the flag, the command reads the current revision and writes against it, which protects only against a change made between that read and the write.
The command never retries a conflict: it prints the current rule and exits with an error.

If you use [audit logs](../admin/security/audit-logs.md), each change creates an entry with the `experiment_rule` resource type.
The entry records who changed the rule and when, but not the condition text.
To read a condition, list the rules.

#### Condition variables

A condition is a CEL expression that returns a boolean.
It can read one variable, `user`, with these fields:

| Field                | Type         | Value                                                                                                             |
|----------------------|--------------|-------------------------------------------------------------------------------------------------------------------|
| `user.id`            | string       | The user's ID                                                                                                     |
| `user.username`      | string       | The user's username                                                                                               |
| `user.email`         | string       | The user's email address, compared case-sensitively                                                               |
| `user.roles`         | list(string) | The user's explicit site roles, such as `owner`. The implied member role and organization roles are not included. |
| `user.organizations` | list(string) | Names of the organizations the user belongs to                                                                    |
| `user.groups`        | list(string) | Groups the user belongs to, as `<organization>/<group>`, including `<organization>/Everyone`                      |

A condition can be at most 4,096 characters long, and each evaluation has a CEL cost limit of 10,000.
Coder rejects a condition that fails to compile and returns the error to the person who wrote it.
Server logs record only the experiment, the rule revision, the error category, and the line and column, never the condition text.

#### When a change takes effect

On supporting replicas, a fresh authoritative database read after the update commits observes the new rule.
In-flight evaluations and work already using an earlier decision are not cancelled.
All serving replicas must run a supporting version before you rely on this control.

Replicas that run an earlier version ignore rules and use their startup `--experiments` list.
Every replica also falls back to its own startup list for experiments without a rule or with `inherit`, so give all replicas the same `--experiments` value.

The dashboard fetches the enabled experiments again when a page loads, when the browser window regains focus, and when the connection is restored, if its copy is more than 60 seconds old.
It doesn't refresh on a timer, so a tab that stays focused can show an out-of-date view.
The server applies the current rule to each request regardless of what the dashboard shows.

#### Failure behavior

If Coder can't read the rules, a stored rule is malformed, or a condition fails to compile or evaluate, the experiment is off for that decision, even if it is enabled at startup.
You can't change this behavior.

Rules gate features; they are not an authorization boundary.
Don't use a rule to restrict access to data or actions.

## Beta

- **Stable**: No
- **Production-ready**: Not fully
- **Support**: Documentation, [Discord](https://discord.gg/coder), and
  [GitHub issues](https://github.com/coder/coder/issues)

Beta features are open to the public and are tagged with a `Beta` label.

They’re in active development and subject to minor changes. They might contain
minor bugs, but are generally ready for use.

Beta features are often ready for general availability within two-three
releases. You should test beta features in staging environments. You can use
beta features in production, but should set expectations and inform users that
some features may be incomplete.

We keep documentation about beta features up-to-date with the latest
information, including planned features, limitations, and workarounds. If you
encounter an issue, please contact your
[Coder account team](https://coder.com/contact), reach out on
[Discord](https://discord.gg/coder), or create a
[GitHub issues](https://github.com/coder/coder/issues) if there isn't one
already. While we will do our best to provide support with beta features, most
issues will be escalated to the product team. Beta features are not covered
within service-level agreements (SLA).

Most beta features are enabled by default. Beta features are announced through
the [Coder Changelog](https://coder.com/changelog), and more information is
available in the documentation.

## General Availability (GA)

- **Stable**: Yes
- **Production-ready**: Yes
- **Support**: Yes, [based on license](https://coder.com/pricing).

All features that are not explicitly tagged as `Early access` or `Beta` are considered generally available (GA).
They have been tested, are stable, and are enabled by default.

If your Coder license includes an SLA, please consult it for an outline of
specific expectations.

For support, consult our knowledgeable and growing community on
[Discord](https://discord.gg/coder), or create a
[GitHub issue](https://github.com/coder/coder/issues) if one doesn't exist
already. Customers with a valid Coder license, can submit a support request or
contact your [account team](https://coder.com/contact).

We intend [Coder documentation](https://github.com/coder/coder/blob/main/contributing/documentation.md) to be the
[single source of truth](https://en.wikipedia.org/wiki/Single_source_of_truth)
and all features should have some form of complete documentation that outlines
how to use or implement a feature. If you discover an error or if you have a
suggestion that could improve the documentation, please
[submit a GitHub issue](https://github.com/coder/internal/issues/new?title=request%28docs%29%3A+request+title+here&labels=["customer-feedback","docs"]&body=please+enter+your+request+here).

Some GA features can be disabled for air-gapped deployments. Consult the
feature's documentation or submit a support ticket for assistance.
