# e2e

The structure of the end-to-end tests is optimized for speed and reliability.
Not all tests require setting up a new PostgreSQL instance or using the
Terraform provisioner. Deciding when to trade time for robustness rests with the
developers; the framework's role is to facilitate this process.

Take a look at prior art in `tests/` for inspiration. To run a test:

```shell
cd site
# Build the frontend assets. If you are actively changing
# the site to debug an issue, add `--watch`.
pnpm build
# Alternatively, build with debug info and source maps:
NODE_ENV=development pnpm vite build --mode=development
# Install the browsers to `~/.cache/ms-playwright`.
pnpm playwright:install
# Run E2E tests. You can see the configuration of the server
# in `playwright.config.ts`. This uses `go run -tags embed ...`.
pnpm playwright:test
# Run a specific test (`-g` stands for grep. It accepts regex).
pnpm playwright:test -g '<your test here>'
```

## Using nix

If this breaks, it is likely because the flake chromium version and playwright
are no longer compatible. To fix this, update the flake to get the latest
chromium version, and adjust the playwright version in the package.json.

You can see the playwright version here:
https://search.nixos.org/packages?channel=unstable&show=playwright-driver&from=0&size=50&sort=relevance&type=packages&query=playwright-driver

```shell
# Optionally add '--command zsh' to choose your shell.
nix develop
cd site
pnpm install
pnpm build
pnpm playwright:test
```

To run the playwright debugger from VSCode, just launch VSCode from the nix
environment and have the extension installed.

```shell
# Optionally add '--command zsh' to choose your shell.
nix develop
code .
```

## Enterprise tests

Enterprise tests require a license key to run.

```shell
export CODER_E2E_LICENSE=<license key>
```

## Debugging tests

To debug a test, it is more helpful to run it in `ui` mode.

```shell
pnpm playwright:test-ui
```
