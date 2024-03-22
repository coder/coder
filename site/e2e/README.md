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
# Install the browsers to `~/.cache/ms-playwright`.
pnpm playwright:install
# Run E2E tests. You can see the configuration of the server
# in `playwright.config.ts`. This uses `go run -tags embed ...`.
pnpm playwright:test
# Run a specific test (`-g` stands for grep. It accepts regex).
pnpm playwright:test -g '<your test here>'
```
