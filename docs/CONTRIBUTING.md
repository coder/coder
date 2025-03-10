# Contributing

## Requirements

<div class="tabs">

We recommend that you use [Nix](https://nix.dev/) package manager to
[maintain dependency versions](https://nixos.org/guides/how-nix-works).

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

Alternatively if you do not want to use Nix then you'll need to install the need
the following tools by hand:

- Go 1.18+
  - on macOS, run `brew install go`
- Node 14+
  - on macOS, run `brew install node`
- GNU Make 4.0+
  - on macOS, run `brew install make`
- [`shfmt`](https://github.com/mvdan/sh#shfmt)
  - on macOS, run `brew install shfmt`
- [`nfpm`](https://nfpm.goreleaser.com/install)
  - on macOS, run `brew install goreleaser/tap/nfpm && brew install nfpm`
- [`pg_dump`](https://stackoverflow.com/a/49689589)
  - on macOS, run `brew install libpq zstd`
  - on Linux, install [`zstd`](https://github.com/horta/zstd.install)
- PostgreSQL 13 (optional if Docker is available)
  - *Note*: If you are using Docker, you can skip this step
  - on macOS, run `brew install postgresql@13` and `brew services start postgresql@13`
  - To enable schema generation with non-containerized PostgreSQL, set the following environment variable:
    - `export DB_DUMP_CONNECTION_URL="postgres://postgres@localhost:5432/postgres?password=postgres&sslmode=disable"`
- `pkg-config`
  - on macOS, run `brew install pkg-config`
- `pixman`
  - on macOS, run `brew install pixman`
- `cairo`
  - on macOS, run `brew install cairo`
- `pango`
  - on macOS, run `brew install pango`
- `pandoc`
  - on macOS, run `brew install pandocomatic`

</div>

## Development workflow

Use the following `make` commands and scripts in development:

- `./scripts/develop.sh` runs the frontend and backend development server
- `make build` compiles binaries and release packages
- `make install` installs binaries to `$GOPATH/bin`
- `make test`

### Running Coder on development mode

- Run `./scripts/develop.sh`
- Access `http://localhost:8080`
- The default user is `admin@coder.com` and the default password is
  `SomeSecurePassword!`

### Running Coder using docker-compose

This mode is useful for testing HA or validating more complex setups.

- Generate a new image from your HEAD: `make build/coder_$(./scripts/version.sh)_$(go env GOOS)_$(go env GOARCH).tag`
  - This will output the name of the new image, e.g.: `ghcr.io/coder/coder:v2.19.0-devel-22fa71d15-amd64`
- Inject this image into docker-compose: `CODER_VERSION=v2.19.0-devel-22fa71d15-amd64 docker-compose up` (*note the prefix `ghcr.io/coder/coder:` was removed*)
- To use Docker, determine your host's `docker` group ID with `getent group docker | cut -d: -f3`, then update the value of `group_add` and uncomment

### Deploying a PR

You need to be a member or collaborator of the [coder](https://github.com/coder) GitHub organization to be able to deploy a PR.

You can test your changes by creating a PR deployment. There are two ways to do
this:

- Run `./scripts/deploy-pr.sh`
- Manually trigger the
  [`pr-deploy.yaml`](https://github.com/coder/coder/actions/workflows/pr-deploy.yaml)
  GitHub Action workflow:

  <Image src="./images/deploy-pr-manually.png" alt="Deploy PR manually" height="348px" align="center" />

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

### Adding database migrations and fixtures

#### Database migrations

Database migrations are managed with
[`migrate`](https://github.com/golang-migrate/migrate).

To add new migrations, use the following command:

```shell
./coderd/database/migrations/create_migration.sh my name
/home/coder/src/coder/coderd/database/migrations/000070_my_name.up.sql
/home/coder/src/coder/coderd/database/migrations/000070_my_name.down.sql
```

Then write queries into the generated `.up.sql` and `.down.sql` files and commit
them into the repository. The down script should make a best-effort to retain as
much data as possible.

Run `make gen` to generate models.

#### Database fixtures (for testing migrations)

There are two types of fixtures that are used to test that migrations don't
break existing Coder deployments:

- Partial fixtures
  [`migrations/testdata/fixtures`](../coderd/database/migrations/testdata/fixtures)
- Full database dumps
  [`migrations/testdata/full_dumps`](../coderd/database/migrations/testdata/full_dumps)

Both types behave like database migrations (they also
[`migrate`](https://github.com/golang-migrate/migrate)). Their behavior mirrors
Coder migrations such that when migration number `000022` is applied, fixture
`000022` is applied afterwards.

Partial fixtures are used to conveniently add data to newly created tables so
that we can ensure that this data is migrated without issue.

Full database dumps are for testing the migration of fully-fledged Coder
deployments. These are usually done for a specific version of Coder and are
often fixed in time. A full database dump may be necessary when testing the
migration of multiple features or complex configurations.

To add a new partial fixture, run the following command:

```shell
./coderd/database/migrations/create_fixture.sh my fixture
/home/coder/src/coder/coderd/database/migrations/testdata/fixtures/000070_my_fixture.up.sql
```

Then add some queries to insert data and commit the file to the repo. See
[`000024_example.up.sql`](../coderd/database/migrations/testdata/fixtures/000024_example.up.sql)
for an example.

To create a full dump, run a fully fledged Coder deployment and use it to
generate data in the database. Then shut down the deployment and take a snapshot
of the database.

```shell
mkdir -p coderd/database/migrations/testdata/full_dumps/v0.12.2 && cd $_
pg_dump "postgres://coder@localhost:..." -a --inserts >000069_dump_v0.12.2.up.sql
```

Make sure sensitive data in the dump is desensitized, for instance names,
emails, OAuth tokens and other secrets. Then commit the dump to the project.

To find out what the latest migration for a version of Coder is, use the
following command:

```shell
git ls-files v0.12.2 -- coderd/database/migrations/*.up.sql
```

This helps in naming the dump (e.g. `000069` above).

## Styling

### Documentation

Visit our [documentation style guide](./contributing/documentation.md).

### Backend

#### Use Go style

Contributions must adhere to the guidelines outlined in
[Effective Go](https://go.dev/doc/effective_go). We prefer linting rules over
documenting styles (run ours with `make lint`); humans are error-prone!

Read
[Go's Code Review Comments Wiki](https://github.com/golang/go/wiki/CodeReviewComments)
for information on common comments made during reviews of Go code.

#### Avoid unused packages

Coder writes packages that are used during implementation. It isn't easy to
validate whether an abstraction is valid until it's checked against an
implementation. This results in a larger changeset, but it provides reviewers
with a holistic perspective regarding the contribution.

### Frontend

Our frontend guide can be found [here](./contributing/frontend.md).

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
workflow. They are created based on the current
[`main`](https://github.com/coder/coder/tree/main) branch.

The release notes for a release are automatically generated from commit titles
and metadata from PRs that are merged into `main`.

### Creating a release

The creation of a release is initiated via
[`./scripts/release.sh`](https://github.com/coder/coder/blob/main/scripts/release.sh).
This script will show a preview of the release that will be created, and if you
choose to continue, create and push the tag which will trigger the creation of
the release via GitHub Actions.

See `./scripts/release.sh --help` for more information.

### Creating a release (via workflow dispatch)

Typically the workflow dispatch is only used to test (dry-run) a release,
meaning no actual release will take place. The workflow can be dispatched
manually from
[Actions: Release](https://github.com/coder/coder/actions/workflows/release.yaml).
Simply press "Run workflow" and choose dry-run.

If a release has failed after the tag has been created and pushed, it can be
retried by again, pressing "Run workflow", changing "Use workflow from" from
"Branch: main" to "Tag: vX.X.X" and not selecting dry-run.

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

- Good: `feat(api): add feature X`
- Bad: `feat(api): added feature X` (past tense)

A good rule of thumb for writing good commit messages is to recite:
[If applied, this commit will ...](https://reflectoring.io/meaningful-commit-messages/).

**Note:** We lint PR titles to ensure they follow the Conventional Commits
specification, however, it's still possible to merge PRs on GitHub with a badly
formatted title. Take care when merging single-commit PRs as GitHub may prefer
to use the original commit title instead of the PR title.

### Breaking changes

Breaking changes can be triggered in two ways:

- Add `!` to the commit message title, e.g.
  `feat(api)!: remove deprecated endpoint /test`
- Add the
  [`release/breaking`](https://github.com/coder/coder/issues?q=sort%3Aupdated-desc+label%3Arelease%2Fbreaking)
  label to a PR that has, or will be, merged into `main`.

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

### Nix on macOS: `error: creating directory`

On macOS, a [direnv bug](https://github.com/direnv/direnv/issues/1345) can cause
`nix-shell` to fail to build or run `coder`. If you encounter
`error: creating directory` when you attempt to run, build, or test, add a
`mkdir` line to your `.envrc`:

```shell
use nix
mkdir -p "$TMPDIR"
```
