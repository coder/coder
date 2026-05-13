# RFC: Replace Chromatic with self-hosted reg-viz

## Summary

Use Storybook Vitest to render stories, `@storycap-testrun/browser` to capture PNGs, `reg-suit` to compare them, object storage for baselines and reports, and GitHub Checks for pass or fail status.

Main auto-accepts new baselines. PRs fail on visual diffs until the code changes or a maintainer accepts the diff.

## Current measurements

- 2,549 Storybook snapshots
- 78.5 MiB PNG data
- 84 MiB on disk
- about 9m20s on one runner
- `reg-suit` needs a wrapper because it does not reliably fail CI on diffs by itself
- storycap `metrics` and `retake` checks should be disabled by default because they timeout on dynamic stories

## Capture

Add a visual snapshot mode to the existing Storybook Vitest project.

```sh
cd site
pnpm test:storybook:snapshots
```

Output:

```text
site/test-results/storybook-snapshots/[story-id].png
```

The capture mode should:

- run only when `VISUAL_REGRESSION=true`
- reuse existing stories and play functions
- load fonts like Chromatic does today
- write generated files under `site/test-results/`

## Storage

Store artifacts outside git.

```text
visual-regression/
  baselines/main/latest.json
  baselines/main/<sha>/snapshots/*.png
  prs/<number>/<sha>/actual/*.png
  prs/<number>/<sha>/diff/*.png
  prs/<number>/<sha>/report/index.html
  accepted/pr/<number>/<sha>/snapshots/*.png
```

Prefer S3-compatible storage. If the chosen storage is not S3-compatible, add a small upload wrapper or custom `reg-suit` publisher.

## Pull request CI

1. Generate snapshots.
2. Pick the baseline:
   - `accepted/pr/<number>/<sha>` if it exists
   - otherwise `baselines/main/latest`
3. Run `reg-suit`.
4. Upload actual, diff, and HTML report.
5. Parse `reg-suit` output.
6. Create a GitHub Check:
   - success when there are no diffs
   - failure when visual diffs exist
   - failure when capture or comparison fails
7. Link the HTML report from the check.

Rejecting a visual change means doing nothing. The check stays failed.

## Main CI

1. Generate snapshots.
2. Upload them as `baselines/main/<sha>`.
3. Update `baselines/main/latest.json`.
4. Pass unless capture fails.

This replaces Chromatic `autoAcceptChanges`.

## Accepting PR diffs

Add an authenticated command, for example:

```text
/accept-visual
```

When a maintainer runs it:

1. Verify write access.
2. Copy the PR's current actual snapshots to `accepted/pr/<number>/<sha>`.
3. Rerun or update the GitHub Check.
4. Pass only for that exact head SHA.

If the PR changes again, it needs review again.

## Performance

Single-run tuning did not help much:

- `maxWorkers=4`: 90s for a 245-story subset
- `maxWorkers=8`: 88s
- `fullPage=false`: 90s
- disabling Vite checker: 88s

Use CI sharding for real speedups:

```sh
STORYBOOK=true VISUAL_REGRESSION=true pnpm vitest run --project=storybook --shard=1/4
STORYBOOK=true VISUAL_REGRESSION=true pnpm vitest run --project=storybook --shard=2/4
STORYBOOK=true VISUAL_REGRESSION=true pnpm vitest run --project=storybook --shard=3/4
STORYBOOK=true VISUAL_REGRESSION=true pnpm vitest run --project=storybook --shard=4/4
```

Expected full-suite wall time with 4 shards: about 2.5 to 3.5 minutes plus upload time.

For PRs, add changed-story mode:

1. Run all stories when CI or Storybook config changes.
2. Run changed story files directly.
3. Map changed source files to importing story files.
4. Fall back to all stories when uncertain.

## Rollout

1. Land snapshot capture behind `VISUAL_REGRESSION=true`.
2. Upload actual snapshots as CI artifacts.
3. Add object storage upload and download.
4. Add `reg-suit` comparison and HTML report upload.
5. Add the GitHub Check wrapper.
6. Add `/accept-visual`.
7. Run Chromatic and reg-viz together for one week.
8. Remove Chromatic after parity is acceptable.

## Open questions

- Which object storage provider should host artifacts?
- Should PRs block immediately, or start report-only?
- Should default capture be full-page or viewport-only?
- What retention should PR artifacts use?
