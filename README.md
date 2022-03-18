[![coder](https://github.com/coder/coder/actions/workflows/coder.yaml/badge.svg)](https://github.com/coder/coder/actions/workflows/coder.yaml)
[![codecov](https://codecov.io/gh/coder/coder/branch/main/graph/badge.svg?token=TNLW3OAP6G)](https://codecov.io/gh/coder/coder)

# Coder v2

This repository contains source code for Coder V2. Additional documentation:

- [Workspaces V2 RFC](https://www.notion.so/coderhq/b48040da8bfe46eca1f32749b69420dd?v=a4e7d23495094644b939b08caba8e381&p=e908a8cd54804ddd910367abf03c8d0a)

## Directory Structure

- `.github/`: Settings for [Dependabot for updating dependencies](https://docs.github.com/en/code-security/supply-chain-security/customizing-dependency-updates) and [build/deploy pipelines with GitHub Actions](https://docs.github.com/en/actions/reference/workflow-syntax-for-github-actions).
  - [`semantic.yaml`](./github/semantic.yaml): Configuration for [semantic pull requests](https://github.com/apps/semantic-pull-requests)
- `examples`: Example terraform project templates.
- `site`: Front-end UI code.

## Development

### Pre-requisites

- `git`
- `go` version 1.17, with the `GOPATH` environment variable set
- `node`
- `yarn`

### Cloning

- `git clone https://github.com/coder/coder`
- `cd coder`

### Building

- `make build`
- `make install`

The `coder` CLI binary will now be available at `$GOPATH/bin/coder`

### Running

After building, the binaries will be available at:
- `dist/coder_{os}_{arch}/coder`

For the purpose of these steps, an OS of `linux` and an arch of `amd64` is assumed.

To manually run the server and go through first-time set up, run the following commands in separate terminals:
- `dist/coder_linux_amd64/coder daemon` <-- starts the Coder server on port 3000
- `dist/coder_linux_amd64/coder login http://localhost:3000` <-- runs through first-time setup, creating a user and org

You'll now be able to login and access the server.

- `dist/coder_linux_amd64/coder projects create -d /path/to/project`

### Development

- `./develop.sh`

The `develop.sh` script does three things:

- runs `coder daemon` locally on port `3000`
- runs `webpack-dev-server` on port `8080`
- sets up an initial user and organization

This is the recommend flow for working on the front-end, as hot-reload is set up as part of the webpack config.

Note that `./develop.sh` creates a user and allows you to log into the UI, but does not log you into the CLI, which is required for creating a project. Use the `login` command above before the `projects create` command.

While we're working on automating XState typegen, you may need to run `yarn typegen` from `site`.

## Front-End Plan

For the front-end team, we're planning on 2 phases to the 'v2' work:

### Phase 1

Phase 1 is the 'new-wine-in-an-old-bottle' approach - we want to preserve the look and feel (UX) of v1, while testing and validating the market fit of our new v2 provisioner model. This means that we'll preserve Material UI and re-use components from v1 (porting them over to the v2 codebase).

### Phase 2

Phase 2 is the 'new-wine-in-a-new-bottle' - which we can do once we've successfully packaged the new wine in the old bottle.

In other words, once we've validated that the new strategy fits and is desirable for our customers, we'd like to build a new, v2-native UI (leveraging designers on the team to build a first-class experience around the new provisioner model).
