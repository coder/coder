# portabledesktop release embedding

The linux slim `coder` binaries embed a pinned
[portabledesktop](https://github.com/coder/portabledesktop) release so the
built-in workspace desktop works without downloading anything inside the
workspace. Nothing here runs in the workspace; the agent installs the embedded
bytes into `$XDG_CACHE_HOME/coder/portabledesktop/<version>-<digest>/` and
execs them.

- `release.lock` pins the release `version`, the `asset` name prefix
  (`portabledesktop` or `portabledesktop-slim`) and the sha256 of each
  architecture's binary, taken from the release's `SHA256SUMS.txt`.
- `fetch.sh` downloads and verifies one binary into
  `agent/x/agentdesktop/embedded/bin/`. `make` runs it as a dependency of the
  linux slim binaries; `scripts/build_go.sh` then sets the
  `portabledesktop_embed` build tag. Set `CODER_PORTABLEDESKTOP_EMBED=0` to build
  without it.

## Testing an unreleased portabledesktop build

Build the slim binary in a portabledesktop checkout and point the fetch at it:

```shell
(cd ../portabledesktop && make build-slim-linux-amd64 build-slim-linux-arm64)
rm -f agent/x/agentdesktop/embedded/bin/portabledesktop-linux-*
CODER_PORTABLEDESKTOP_LOCAL_DIR=../portabledesktop/pd/dist make fetch-portabledesktop
make build/coder-slim_linux_amd64
```

The local binary is copied without checksum verification, and `fetch.sh`
prints a warning. Delete the copied files to go back to the pinned release.

## Bumping the release

Update `version`, `asset` and both checksums in `release.lock`, plus
`embedded.Version` in `agent/x/agentdesktop/embedded/embedded.go`, then delete
`agent/x/agentdesktop/embedded/bin/portabledesktop-linux-*` so the next build
fetches the new release.
