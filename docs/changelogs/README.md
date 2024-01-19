# Changelogs

These are the changelogs used by [generate_release_notes.sh]https://github.com/coder/coder/blob/main/scripts/release/generate_release_notes.sh) for a release.

These changelogs are currently not kept in sync with GitHub releases. Use [GitHub releases](https://github.com/coder/coder/releases) for the latest information!

## Writing a changelog

Run this command to generate release notes:

```shell
git checkout main; git pull; git fetch --all
export CODER_IGNORE_MISSING_COMMIT_METADATA=1
export BRANCH=main
./scripts/release/generate_release_notes.sh \
  --old-version=v2.6.0 \
  --new-version=v2.7.0 \
  --ref=$(git rev-parse --short "${ref:-origin/$BRANCH}") \
  > ./docs/changelogs/v2.7.0.md
```
