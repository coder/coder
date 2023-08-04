# Changelogs

These are the changelogs used by [get-changelog.sh](https://github.com/coder/coder/blob/main/scripts/release/changelog.sh) for a release.

These changelogs are currently not kept in-sync with GitHub releases. Use [GitHub releases](https://github.com/coder/coder/releases) for the latest information!

## Writing a changelog

Run this command to generate release notes:

```sh
./scripts/release/generate_release_notes.sh \
  --old-version=v0.27.0 \
  --new-version=v0.28.0 \
  --ref=$(git rev-parse --short "${ref:-origin/$branch}") \
  > ./docs/changelogs/v0.28.0.md
```
