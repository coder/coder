# Contributing

## Requirements

We recommend using the [Nix](https://nix.dev/) package manager as it makes any pain related to maintaining dependency versions [just disappear](https://twitter.com/mitchellh/status/1491102567296040961). Once nix [has been installed](https://nixos.org/download.html) the development environment can be _manually instantiated_ through the `nix-shell` command:

```
$ cd ~/code/coder

# https://nix.dev/tutorials/declarative-and-reproducible-developer-environments
$ nix-shell

...
copying path '/nix/store/3ms6cs5210n8vfb5a7jkdvzrzdagqzbp-iana-etc-20210225' from 'https://cache.nixos.org'...
copying path '/nix/store/dxg5aijpyy36clz05wjsyk90gqcdzbam-iana-etc-20220520' from 'https://cache.nixos.org'...
copying path '/nix/store/v2gvj8whv241nj4lzha3flq8pnllcmvv-ignore-5.2.0.tgz' from 'https://cache.nixos.org'...
...
```

If [direnv](https://direnv.net/) is installed and the [hooks are configured](https://direnv.net/docs/hook.html) then the development environment can be _automatically instantiated_ by creating the following `.envrc`, thus removing the need to run `nix-shell` by hand!

```
$ cd ~/code/coder
$ echo "use nix" >.envrc
$ direnv allow
```

Now, whenever you enter the project folder, `direnv` will prepare the environment for you:

```
$ cd ~/code/coder

# https://direnv.net/docs/hook.html
direnv: loading ~/code/coder/.envrc
direnv: using nix
direnv: export +AR +AS +CC +CONFIG_SHELL +CXX +HOST_PATH +IN_NIX_SHELL +LD +NIX_BINTOOLS +NIX_BINTOOLS_WRAPPER_TARGET_HOST_x86_64_unknown_linux_gnu +NIX_BUILD_CORES +NIX_BUILD_TOP +NIX_CC +NIX_CC_WRAPPER_TARGET_HOST_x86_64_unknown_linux_gnu +NIX_CFLAGS_COMPILE +NIX_ENFORCE_NO_NATIVE +NIX_HARDENING_ENABLE +NIX_INDENT_MAKE +NIX_LDFLAGS +NIX_STORE +NM +NODE_PATH +OBJCOPY +OBJDUMP +RANLIB +READELF +SIZE +SOURCE_DATE_EPOCH +STRINGS +STRIP +TEMP +TEMPDIR +TMP +TMPDIR +XDG_DATA_DIRS +buildInputs +buildPhase +builder +cmakeFlags +configureFlags +depsBuildBuild +depsBuildBuildPropagated +depsBuildTarget +depsBuildTargetPropagated +depsHostHost +depsHostHostPropagated +depsTargetTarget +depsTargetTargetPropagated +doCheck +doInstallCheck +mesonFlags +name +nativeBuildInputs +out +outputs +patches +phases +propagatedBuildInputs +propagatedNativeBuildInputs +shell +shellHook +stdenv +strictDeps +system ~PATH

🎉
```

Alternatively if you do not want to use nix then you'll need to install the need the following tools by hand:
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
- [`pkg-config`]()
  - on macOS, run `brew install pkg-config`
- [`pixman`]()
  - on macOS, run `brew install pixman`
- [`cairo`]()
  - on macOS, run `brew install cairo`
- [`pango`]()
  - on macOS, run `brew install pango`
- [`pandoc`]()
  - on macOS, run `brew install pandocomatic`


### Development workflow

Use the following `make` commands and scripts in development:

- `./scripts/develop.sh` runs the frontend and backend development server
- `make build` compiles binaries and release packages
- `make install` installs binaries to `$GOPATH/bin`
- `make test`

### Adding database migrations and fixtures

#### Database migrations

Database migrations are managed with [`migrate`](https://github.com/golang-migrate/migrate).

To add new migrations, use the following command:

```
$ ./coderd/database/migrations/create_fixture.sh my name
/home/coder/src/coder/coderd/database/migrations/000070_my_name.up.sql
/home/coder/src/coder/coderd/database/migrations/000070_my_name.down.sql
Run "make gen" to generate models.
```

Then write queries into the generated `.up.sql` and `.down.sql` files and commit
them into the repository. The down script should make a best-effort to retain as
much data as possible.

#### Database fixtures (for testing migrations)

There are two types of fixtures that are used to test that migrations don't
break existing Coder deployments:

- Partial fixtures [`migrations/testdata/fixtures`](../coderd/database/migrations/testdata/fixtures)
- Full database dumps [`migrations/testdata/full_dumps`](../coderd/database/migrations/testdata/full_dumps)

Both types behave like database migrations (they also [`migrate`](https://github.com/golang-migrate/migrate)). Their behavior mirrors Coder migrations such that when migration
number `000022` is applied, fixture `000022` is applied afterwards.

Partial fixtures are used to conveniently add data to newly created tables so
that we can ensure that this data is migrated without issue.

Full database dumps are for testing the migration of fully-fledged Coder
deployments. These are usually done for a specific version of Coder and are
often fixed in time. A full database dump may be necessary when testing the
migration of multiple features or complex configurations.

To add a new partial fixture, run the following command:

```
$ ./coderd/database/migrations/create_fixture.sh my fixture
/home/coder/src/coder/coderd/database/migrations/testdata/fixtures/000070_my_fixture.up.sql
```

Then add some queries to insert data and commit the file to the repo. See
[`000024_example.up.sql`](../coderd/database/migrations/testdata/fixtures/000024_example.up.sql)
for an example.

To create a full dump, run a fully fledged Coder deployment and use it to
generate data in the database. Then shut down the deployment and take a snapshot
of the database.

```
$ mkdir -p coderd/database/migrations/testdata/full_dumps/v0.12.2 && cd $_
$ pg_dump "postgres://coder@localhost:..." -a --inserts >000069_dump_v0.12.2.up.sql
```

Make sure sensitive data in the dump is desensitized, for instance names,
emails, OAuth tokens and other secrets. Then commit the dump to the project.

To find out what the latest migration for a version of Coder is, use the
following command:

```
$ git ls-files v0.12.2 -- coderd/database/migrations/*.up.sql
```

This helps in naming the dump (e.g. `000069` above).


## Styling

### Documentation

Our style guide for authoring documentation can be found [here](./contributing/documentation.md).

### Backend

#### Use Go style

Contributions must adhere to the guidelines outlined in [Effective
Go](https://go.dev/doc/effective_go). We prefer linting rules over documenting
styles (run ours with `make lint`); humans are error-prone!

Read [Go's Code Review Comments
Wiki](https://github.com/golang/go/wiki/CodeReviewComments) for information on
common comments made during reviews of Go code.

#### Avoid unused packages

Coder writes packages that are used during implementation. It isn't easy to
validate whether an abstraction is valid until it's checked against an
implementation. This results in a larger changeset, but it provides reviewers
with a holistic perspective regarding the contribution.

### Frontend

#### Follow component conventions

Each component gets its own folder. Make sure you add a test and Storybook
stories for the component as well. By keeping these tidy, the codebase will
remain easy-to-navigate, healthy and maintainable for all contributors.

#### Keep accessibility in mind

We strive to keep our UI accessible. When using colors, avoid adding new
elements with low color contrast. Always use labels on inputs, not just
placeholders. These are important for screen-readers.

## Reviews

> The following information has been borrowed from [Go's review
> philosophy](https://go.dev/doc/contribute#reviews).

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
