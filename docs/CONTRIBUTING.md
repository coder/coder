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

If [direnv](https://direnv.net/) is installed and the [hooks are configured](https://direnv.net/docs/hook.html) then the development environment can be _automatically instantiated_ thus removing the need to run `nix-shell` by hand!

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
- Node 14+
- GNU Make
- [`shfmt`](https://github.com/mvdan/sh#shfmt)
- [`nfpm`](https://nfpm.goreleaser.com/install)
- [`pg_dump`]
  - on macOS, run `brew install libpq zstd`
  - on Linux, install [`zstd`](https://github.com/horta/zstd.install)


### Development workflow

Use the following `make` commands and scripts in development:

- `./scripts/develop.sh` runs the frontend and backend development server
- `make build` compiles binaries and release packages
- `make install` installs binaries to `$GOPATH/bin`
- `make test`

## Styling

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
