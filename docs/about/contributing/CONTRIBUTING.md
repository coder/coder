# Contributing

## Requirements

<div class="tabs">

To get started with Coder, the easiest way to set up the required environment is to use the provided [Nix environment](https://github.com/coder/coder/tree/main/nix).
Learn more [how Nix works](https://nixos.org/guides/how-nix-works).

### Nix

1. [Install Nix](https://nix.dev/install-nix#install-nix)

1. After you've installed Nix, instantiate the development with the `nix-shell`
   command:

   ```shell
   cd ~/code/coder

   # https://nix.dev/tutorials/declarative-and-reproducible-developer-environments
   nix-shell

   ...
   copying path '/nix/store/3ms6cs5210n8vfb5a7jkdvzrzdagqzbp-iana-etc-20210225' from 'https://   cache.nixos.org'...
   copying path '/nix/store/dxg5aijpyy36clz05wjsyk90gqcdzbam-iana-etc-20220520' from 'https://   cache.nixos.org'...
   copying path '/nix/store/v2gvj8whv241nj4lzha3flq8pnllcmvv-ignore-5.2.0.tgz' from 'https://cache.   nixos.org'...
   ...
   ```

1. Optional: If you have [direnv](https://direnv.net/) installed with
   [hooks configured](https://direnv.net/docs/hook.html), you can add `use nix`
   to `.envrc` to automatically instantiate the development environment:

   ```shell
   cd ~/code/coder
   echo "use nix" >.envrc
   direnv allow
   ```

   Now, whenever you enter the project folder,
   [`direnv`](https://direnv.net/docs/hook.html) will prepare the environment
   for you:

   ```shell
   cd ~/code/coder

   direnv: loading ~/code/coder/.envrc
   direnv: using nix
   direnv: export +AR +AS +CC +CONFIG_SHELL +CXX +HOST_PATH +IN_NIX_SHELL +LD +NIX_BINTOOLS +NIX_BINTOOLS_WRAPPER_TARGET_HOST_x86_64_unknown_linux_gnu +NIX_BUILD_CORES +NIX_BUILD_TOP +NIX_CC +NIX_CC_WRAPPER_TARGET_HOST_x86_64_unknown_linux_gnu +NIX_CFLAGS_COMPILE +NIX_ENFORCE_NO_NATIVE +NIX_HARDENING_ENABLE +NIX_INDENT_MAKE +NIX_LDFLAGS +NIX_STORE +NM +NODE_PATH +OBJCOPY +OBJDUMP +RANLIB +READELF +SIZE +SOURCE_DATE_EPOCH +STRINGS +STRIP +TEMP +TEMPDIR +TMP +TMPDIR +XDG_DATA_DIRS +buildInputs +buildPhase +builder +cmakeFlags +configureFlags +depsBuildBuild +depsBuildBuildPropagated +depsBuildTarget +depsBuildTargetPropagated +depsHostHost +depsHostHostPropagated +depsTargetTarget +depsTargetTargetPropagated +doCheck +doInstallCheck +mesonFlags +name +nativeBuildInputs +out +outputs +patches +phases +propagatedBuildInputs +propagatedNativeBuildInputs +shell +shellHook +stdenv +strictDeps +system ~PATH

   🎉
   ```

   - If you encounter a `creating directory` error on macOS, check the
     [troubleshooting](#troubleshooting) section below.

### Without Nix

If you're not using the Nix environment, you can launch a local [DevContainer](https://github.com/coder/coder/tree/main/.devcontainer) to get a fully configured development environment.

DevContainers are supported in tools like **VS Code** and **GitHub Codespaces**, and come preloaded with all required dependencies: Docker, Go, Node.js with `pnpm`, and `make`.

</div>

## Development workflow

Use the following `make` commands and scripts in development:

- `./scripts/develop.sh` runs the frontend and backend development server
- `make build` compiles binaries and release packages
- `make install` installs binaries to `$GOPATH/bin`
- `make test`
- `make pre-commit` runs gen, fmt, lint, typos, and builds a slim binary
- `make pre-commit-light` runs fmt and lint for shell, terraform, markdown,
  helm, actions, and typos (skips gen, Go/TS lint+fmt, and binary build)
- `make pre-push` runs heavier CI checks including tests (allowlisted)

Install the git hooks to run these automatically:

```sh
git config core.hooksPath scripts/githooks
```

The hooks classify staged/changed files and select the appropriate target.
Commits that only touch docs, shell, terraform, or other lightweight files
run `make pre-commit-light` instead of the full `make pre-commit`, and
`pre-push` is skipped entirely. Changes to Go, TypeScript, SQL, proto, or
the Makefile trigger the full targets as before.

### Running Coder on development mode

1. Run the development script to spin up the local environment:

   ```sh
   ./scripts/develop.sh
   ```

   This will start two processes:

   - http://localhost:3000 — the backend API server. Primarily used for backend development and also serves the *static* frontend build.
   - http://localhost:8080 — the Node.js frontend development server. Supports *hot reloading* and is useful if you're working on the frontend as well.

   Additionally, it starts a local PostgreSQL instance, creates both an admin and a member user account, and installs a default Docker-based template.

1. Verify Your Session

   Confirm that you're logged in by running:

   ```sh
   ./scripts/coder-dev.sh list
      ```

   This should return an empty list of workspaces. If you encounter an error, review the output from the [develop.sh](https://github.com/coder/coder/blob/main/scripts/develop.sh) script for issues.

   > [!NOTE]
   > `coder-dev.sh` is a helper script that behaves like the regular coder CLI, but uses the binary built from your local source and shares the same configuration directory set up by `develop.sh`. This ensures your local changes are reflected when testing.
   >
   > The default user is `admin@coder.com` and the default password is `SomeSecurePassword!`

1. Create Your First Workspace

   A template named `docker` is created automatically. To spin up a workspace quickly, use:

   ```sh
   ./scripts/coder-dev.sh create my-workspace -t docker
   ```

### Deploying a PR

You need to be a member or collaborator of the [coder](https://github.com/coder) GitHub organization to be able to deploy a PR.

You can test your changes by creating a PR deployment. There are two ways to do
this:

- Run `./scripts/deploy-pr.sh`
- Manually trigger the
  [`pr-deploy.yaml`](https://github.com/coder/coder/actions/workflows/pr-deploy.yaml)
  GitHub Action workflow.

#### Available options

- `-d` or `--deploy`, force deploys the PR by deleting the existing deployment.
- `-b` or `--build`, force builds the Docker image. (generally not needed as we
  are intelligently checking if the image needs to be built)
- `-e EXPERIMENT1,EXPERIMENT2` or `--experiments EXPERIMENT1,EXPERIMENT2`, will
  enable the specified experiments. (defaults to `*`)
- `-n` or `--dry-run` will display the context without deployment. e.g., branch
  name and PR number, etc.
- `-y` or `--yes`, will skip the CLI confirmation prompt.

> [!NOTE]
> PR deployment will be re-deployed automatically when the PR is updated.
> It will use the last values automatically for redeployment.

Once the deployment is finished, a unique link and credentials will be posted in
the [#pr-deployments](https://codercom.slack.com/archives/C05DNE982E8) Slack
channel.

## Styling

- [Documentation style guide](./documentation.md)

- [Frontend styling guide](./frontend.md#styling)

## Pull Requests

We welcome pull requests (PRs) from community members including (but not limited to) open source users, enthusiasts, and enterprise customers.

We will ask that you sign a Contributor License Agreement before we accept any contributions into our repo.

Please keep PRs small and self-contained. This allows code reviewers (see below) to focus and fully understand the PR. A good rule of thumb is less than 1000 lines changed. (One exception is a mechanistic refactor, like renaming, that is conceptually trivial but might have a large line count.)

If your intended feature or refactor will be larger than this:

 1. Open an issue explaining what you intend to build, how it will work, and that you are volunteering to do the development. Include `@coder/community-triage` in the body.
 2. Give the maintainers a chance to respond. Changes to the visual, interaction, or software design are easier to adjust before you start laying down code.
 3. Break your work up into a series of smaller PRs.

Stacking tools like [Graphite](https://www.graphite.dev) are useful for keeping a series of PRs that build on each other up to date as they are reviewed and merged.

Each PR:

- Must individually build and pass all tests, including formatting and linting.
- Must not introduce regressions or backward-compatibility issues, even if a subsequent PR in your series would resolve the issue.
- Should be a conceptually coherent change set.

In practice, many of these smaller PRs will be invisible to end users, and that is ok. For example, you might introduce
a new Go package that implements the core business logic of a feature in one PR, but only later actually "wire it up"
to a new API route in a later PR. Or, you might implement a new React component in one PR, and only in a later PR place it on a page.

## Reviews

The following information has been borrowed from [Go's review philosophy](https://go.dev/doc/contribute#reviews).

Coder values thorough reviews. For each review comment that you receive, please
"close" it by implementing the suggestion or providing an explanation on why the
suggestion isn't the best option. Be sure to do this for each comment; you can
click **Done** to indicate that you've implemented the suggestion, or you can
add a comment explaining why you aren't implementing the suggestion (or what you
chose to implement instead).

It is perfectly normal for changes to go through several rounds of reviews, with
one or more reviewers making new comments every time, then waiting for an
updated change before reviewing again. All contributors, including those from
maintainers, are subject to the same review cycle; this process is not meant to
be applied selectively or to discourage anyone from contributing.

## Releases

Coder releases are initiated via
[`./scripts/release.sh`](https://github.com/coder/coder/blob/main/scripts/release.sh)
and automated via GitHub Actions. Specifically, the
[`release.yaml`](https://github.com/coder/coder/blob/main/.github/workflows/release.yaml)
workflow.

Release notes are automatically generated from commit titles and PR metadata.

### Release types

| Type                   | Tag           | Branch        | Purpose                                 |
|------------------------|---------------|---------------|-----------------------------------------|
| RC (release candidate) | `vX.Y.0-rc.W` | `main`        | Ad-hoc pre-release for customer testing |
| Release                | `vX.Y.0`      | `release/X.Y` | First release of a minor version        |
| Patch                  | `vX.Y.Z`      | `release/X.Y` | Bug fixes and security patches          |

### Workflow

RC tags are created directly on `main`. The `release/X.Y` branch is only cut
when the release is ready. This avoids cherry-picking main's progress onto
a release branch between the first RC and the release.

```text
main:  ──●──●──●──●──●──●──●──●──●──
              ↑           ↑     ↑
           rc.0        rc.1    cut release/2.34, tag v2.34.0
                                     \
                               release/2.34:  ──●── v2.34.1 (patch)
```

1. **RC:** On `main`, run `./scripts/release.sh`. The tool suggests the next
   RC version and tags it on `main`.
2. **Release:** When the RC is blessed, create `release/X.Y` from `main` (or
   the specific RC commit). Switch to that branch and run
   `./scripts/release.sh`, which suggests `vX.Y.0`.
3. **Patch:** Cherry-pick fixes onto `release/X.Y` and run
   `./scripts/release.sh` from that branch.

The release tool warns if you try to tag a non-RC on `main` or an RC on a
release branch.

### Creating a release (via workflow dispatch)

If the
[`release.yaml`](https://github.com/coder/coder/actions/workflows/release.yaml)
workflow fails after the tag has been pushed, retry it from the GitHub Actions
UI: press "Run workflow", set "Use workflow from" to the tag (e.g.
`Tag: v2.34.0`), select the correct release channel, and do **not** select
dry-run.

To test the workflow without publishing, select dry-run.

### Commit messages

Commit messages should follow the
[Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/)
specification.

Allowed commit types (`feat`, `fix`, etc.) are listed in
[conventional-commit-types](https://github.com/commitizen/conventional-commit-types/blob/c3a9be4c73e47f2e8197de775f41d981701407fb/index.json).
Note that these types are also used to automatically sort and organize the
release notes.

A good commit message title uses the imperative, present tense and is ~50
characters long (no more than 72).

Examples:

- Good: `feat(coderd): add feature X`
- Bad: `feat(coderd): added feature X` (past tense)

Scopes must reference a real path in the repository (a directory or file stem)
and must contain all changed files. For example, use `coderd/database` if all
changes are within that directory. If changes span multiple top-level
directories, omit the scope.

A good rule of thumb for writing good commit messages is to recite:
[If applied, this commit will ...](https://reflectoring.io/meaningful-commit-messages/).

**Note:** We lint PR titles to ensure they follow the Conventional Commits
specification, however, it's still possible to merge PRs on GitHub with a badly
formatted title. Take care when merging single-commit PRs as GitHub may prefer
to use the original commit title instead of the PR title.

### Backporting fixes to release branches

When a merged PR on `main` should also ship in older releases, add the
`backport` label to the PR. The
[backport workflow](https://github.com/coder/coder/blob/main/.github/workflows/backport.yaml)
will automatically detect the latest three `release/*` branches,
cherry-pick the merge commit onto each one, and open PRs for
review.

The label can be added before or after the PR is merged. Each backport
PR reuses the original title (e.g.
`fix(site): correct button alignment (#12345)`) so the change is
meaningful in release notes.

If the cherry-pick encounters conflicts, the backport PR is still created
with instructions for manual resolution — no conflict markers are committed.

### Breaking changes

Breaking changes can be triggered in two ways:

- Add `!` to the commit message title, e.g.
  `feat(coderd)!: remove deprecated endpoint /test`
- Add the
  [`release/breaking`](https://github.com/coder/coder/issues?q=sort%3Aupdated-desc+label%3Arelease%2Fbreaking)
  label to a PR that has, or will be, merged into `main`.

### Generative AI

Using AI to help with contributions is acceptable, but only if the [AI Contribution Guidelines](./AI_CONTRIBUTING.md)
are followed. If most of your PR was generated by AI, please read and comply with these rules before submitting.

### Security

> [!CAUTION]
> If you find a vulnerability, **DO NOT FILE AN ISSUE**. Instead, send an email
> to <security@coder.com>.

The
[`security`](https://github.com/coder/coder/issues?q=sort%3Aupdated-desc+label%3Asecurity)
label can be added to PRs that have, or will be, merged into `main`. Doing so
will make sure the change stands out in the release notes.

### Experimental

The
[`release/experimental`](https://github.com/coder/coder/issues?q=sort%3Aupdated-desc+label%3Arelease%2Fexperimental)
label can be used to move the note to the bottom of the release notes under a
separate title.

## Troubleshooting

### Database migration mismatch after switching branches

If `./scripts/develop.sh` exits with a "database migration conflict" error,
it means the database has migrations from another branch that don't exist
on the current one. You have two options:

```shell
# Roll back the mismatched migrations (preserves your dev data):
./scripts/develop.sh --db-rollback

# Or wipe the database and start fresh:
./scripts/develop.sh --db-reset
```

### Nix on macOS: `error: creating directory`

On macOS, a [direnv bug](https://github.com/direnv/direnv/issues/1345) can cause
`nix-shell` to fail to build or run `coder`. If you encounter
`error: creating directory` when you attempt to run, build, or test, add a
`mkdir` line to your `.envrc`:

```shell
use nix
mkdir -p "$TMPDIR"
```
