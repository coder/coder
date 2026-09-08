# Desktop runtime

This directory builds `desktop-runtime-linux-<arch>.tar.zst`, a minimal
self-contained X server tree that the Coder workspace agent embeds and unpacks
at runtime to serve a browser based desktop session.

The runtime contains exactly what is needed to run a headless X session that a
noVNC client can connect to:

- `Xvnc` from TigerVNC, built inside an xorg-server source tree.
- `xkbcomp`, which the X server executes to compile the keymap.
- A trimmed `xkeyboard-config` data set covering the default `us` / `pc105` /
  `evdev` keymap.

Both binaries are compiled against musl on Alpine and linked **fully static**,
so the tree runs on any Linux distribution and does not depend on the workspace
image having X libraries, a matching libc, or anything else installed.

## Where Docker fits (and where it does not)

Docker is used **only by the `desktop-runtime` GitHub workflow** (and by
developers iterating on the `Dockerfile`), as a cross-compilation environment
that produces the archive. It is not part of the Coder binary build and not part
of the workspace runtime path:

1. When a file in this directory changes on `main`, the workflow runs `build.sh`
   for both architectures and publishes the archives to
   `https://releases.coder.com/desktop-runtime/<version>/`, where `<version>`
   is the digest of the build inputs printed by `version.sh`.
2. `runtime.lock` pins that version and the sha256 of each archive. `make`
   runs `fetch.sh`, which downloads the pinned archive and verifies it, and the
   Go build embeds it. The binary build therefore stays pure Go plus one
   checksummed download; `ci.yaml` and `release.yaml` never run a container.
3. In a workspace, the agent unpacks the embedded archive to a directory it owns
   and execs `bin/Xvnc` directly.

So inside the workspace there is **no Docker, no network access, no package
manager, and no dependency on host libraries**. The workspace image can be any
Linux distribution, with any libc or none of the X stack installed, and the
running user needs no privileges beyond writing to its own unpack directory.

This is why fully static linking is a hard preference rather than a nice to
have: a static binary has no loader, no `RPATH`, no `LD_LIBRARY_PATH` handling
and no interaction with whatever happens to be in `/lib` or `/usr/lib` in the
workspace. The bundled `ld-musl-<arch>.so.1` plus `.so` closure fallback exists
only for the case where static linking is genuinely infeasible; it is not in use
today (see [Static linking](#static-linking)). If that fallback ever has to be
taken, it must be validated on a glibc host, because that is the case it exists
to survive: unpack the archive on a non-Alpine machine and confirm that
`bin/Xvnc` starts, serves RFB and compiles its keymap there, not just inside the
Alpine build image.

## Usage

Normal builds never call these scripts directly: `make build` (or any linux
binary target) runs `fetch.sh` for the pinned archive when `runtime.lock` is
populated. Set `CODER_DESKTOP_RUNTIME=0` to build without the runtime, or
`CODER_DESKTOP_RUNTIME_URL` to download from a mirror.

To rebuild the archives locally, for example while changing the `Dockerfile`:

```console
$ make build-desktop-runtime
```

or for a single architecture:

```console
$ ./scripts/desktopruntime/build.sh --arch amd64 --output /tmp/desktop-runtime-linux-amd64.tar.zst
$ ./scripts/desktopruntime/check_size.sh /tmp/desktop-runtime-linux-amd64.tar.zst
```

`build.sh` runs the build command (`docker buildx build` by default) with
`--platform linux/<arch>` and exports the scratch stage directly to a directory,
then packs it with a fixed sort order, timestamp and ownership before
compressing with `zstd -19 -T0`. The build image also sets `SOURCE_DATE_EPOCH`,
so two independent `--no-cache` builds of the same sources produce byte
identical archives, including across different builders. `check_size.sh` fails
when the archive exceeds its budget (6 MiB by default).

### Choosing a builder

The build needs a builder that can export a filesystem (`--output type=local`).
Nothing in the `Dockerfile` touches the local Docker daemon, so a remote builder
works as well as a local one; the builder does need outbound network access to
fetch the pinned sources.

| Selection | Behavior |
|---|---|
| `--builder <name>` | Passed straight through to the build command |
| `BUILDX_BUILDER` in the environment | Left alone for buildx to pick up |
| Neither | Uses the current builder, and only falls back to creating a local `coder-desktopruntime` docker-container builder when that builder cannot export `type=local` |

The fallback is decided by a real capability probe (exporting an empty `FROM
scratch` image, which needs no image pull and takes under 100 ms) rather than by
matching on the driver name. Current Docker daemons export `type=local` fine, so
the fallback normally does not trigger.

### Using Depot in CI

`CODER_DESKTOP_RUNTIME_BUILDX` replaces the build command, which defaults to
`docker buildx build`. The `desktop-runtime` workflow sets it to the Depot CLI,
which accepts the same `--platform` and `--output` flags:

```console
$ CODER_DESKTOP_RUNTIME_BUILDX="depot build --project wl5hnrrkns" \
    ./scripts/desktopruntime/build.sh --arch arm64 --output /tmp/desktop-runtime-linux-arm64.tar.zst
```

When the command is overridden, no builder is injected and the capability probe
is skipped, so nothing docker specific is assumed. Depot builds arm64 on native
arm64 nodes, so CI needs no emulation at all.

### QEMU (local and fork builds only)

Without Depot, building `linux/arm64` on an x86 host needs QEMU:

```shell
docker run --rm --privileged tonistiigi/binfmt --install arm64
```

This is the fallback path for local development and fork builds. It works, but
the whole Alpine toolchain runs emulated, which costs roughly 11x.

### Measured cold build times

No cache, on a 128 core host:

| Target | Buildkit step time | Wall clock | Notes |
|---|---|---|---|
| `linux/amd64` | 57 s | 63 to 74 s | native |
| `linux/arm64` | 665 s (11.1 min) | 685 s (11.4 min) | QEMU, 11.6x slower |

The emulation cost sits in the compile steps: the xorg-server configure, build
and relink goes from 22 s to 295 s, and the static helper libraries go from 13 s
to 252 s. Downloads and `apk add` are barely affected.

Smaller CI runners scale both numbers up, which is why the runtime is built and
published only when its inputs change and the binary build downloads the pinned
result. Depot for arm64 or a native arm64 runner removes the emulation penalty
entirely.

## Output layout

```text
bin/Xvnc            statically linked TigerVNC X server
bin/xkbcomp         statically linked keymap compiler
share/xkb/...       trimmed xkeyboard-config data (rules, keycodes, types,
                    compat, symbols, geometry)
manifest.json       schema version, target architecture, component versions
                    and whether the binaries are static
LICENSES/           license text of every bundled component
```

## Relocatability and how to launch it

The tree is unpacked to an arbitrary directory, so `Xvnc` must not need any
build-time absolute path at run time. Three configure options guarantee that:

| Path | Configure option | Runtime behavior |
|---|---|---|
| xkbcomp | `--with-xkb-bin-directory=` (empty) | The server runs plain `xkbcomp`, resolved through `PATH` |
| XKB data | `--with-xkb-path=/usr/share/X11/xkb` | Compiled in, but always overridden with `-xkbdir` |
| Fonts | `--with-default-font-path=built-ins` | The libXfont2 built-in fonts live inside the binary, so no font directory has to exist |

The compiled keymap is written to `/tmp` (`--with-xkb-output=/tmp`), so the
unpacked runtime directory itself never has to be writable.

The launcher therefore has exactly two obligations: prepend the runtime's `bin`
directory to `PATH`, and pass `-xkbdir`. This is the invocation that was
verified on a glibc host:

```console
$ PATH="$runtime/bin:$PATH" "$runtime/bin/Xvnc" :77 \
    -rfbport 5977 \
    -localhost \
    -SecurityTypes None \
    -AlwaysShared \
    -AcceptSetDesktopSize \
    -geometry 1280x800 \
    -depth 24 \
    -xkbdir "$runtime/share/xkb" \
    -desktop Coder
```

No `-fp` flag is needed; passing `-fp built-ins` explicitly is equivalent. Both
variants expose the same six built-in core fonts (`fixed`, `cursor`, `6x13` and
three aliases), which is enough for legacy core-font clients to start.
Applications that render text client side through fontconfig and Xft use the
fonts present in the workspace image and are unaffected.

Both obligations are hard requirements, not best effort:

- Without the runtime's `bin` on `PATH` the server logs `xkbcomp: not found`,
  then `XKB: Failed to compile keymap`, and exits with
  `Fatal server error: Failed to activate virtual core keyboard`.
- Without `-xkbdir` the server falls back to the compiled-in
  `/usr/share/X11/xkb`. On a host that has no xkeyboard-config installed this is
  the same fatal failure; on a host that does have it, the server silently
  compiles the keymap from the host's data instead of ours, which is exactly the
  behavior this runtime exists to avoid.

## What is excluded and why

The runtime is embedded in the agent binary, so every megabyte is paid for on
every agent download. The exclusions below are what keep it under 2 MiB
compressed.

- **GLX, Mesa, DRI, glamor, libdrm.** By far the largest lever. Enabling GLX
  pulls in Mesa, which pulls in LLVM for its software rasterizer, adding
  roughly 180 MB of installed size on Alpine. A VNC session composites in
  software on the client side, so the server needs no GL at all.
- **GnuTLS and nettle.** `Xvnc` listens on localhost only with
  `-SecurityTypes None`. The Coder agent terminates and authenticates the
  connection before any traffic reaches the X server, so in-server TLS and
  RSA-AES authentication would only duplicate work already done.
- **PAM.** For the same reason there is no in-server password check. PAM also
  makes a static binary impossible because it `dlopen`s its modules at runtime.
  TigerVNC's CMake build requires PAM development files unconditionally, so
  `pam_stub.c` supplies a small archive whose entry points all fail closed, and
  the real shared library is removed from the build image.
- **ffmpeg and H.264.** noVNC decodes Tight, ZRLE and JPEG in the browser, so
  the H.264 encoder and its libav dependencies are dead weight.
- **systemd, SELinux, wayland, pwquality, NLS.** Session management, labelling
  and translations are not used by an agent-managed server.
- **The TigerVNC viewer, x0vncserver and vncsession.** Only the `Xvnc` binary
  and the libraries it links are built.
- **FreeType in libXfont2.** The runtime ships no fonts, so the scalable font
  backend (and with it libpng, brotli and bzip2) buys nothing. The built-in and
  PCF bitmap backends remain, which is what the X server needs to start.
  Applications render text client side through fontconfig and Xft using the
  fonts present in the workspace image.
- **Most of xkeyboard-config.** Only the `evdev` rules file, the `evdev` and
  `aliases` keycodes, all types and compat files, the symbol files reachable
  from `pc+us+inet(evdev)`, and the `pc` geometry are kept. The `*.xml` and
  `*.lst` catalogues only feed configuration GUIs. This reduces about 10 MB of
  data to 580 KB. The build compiles the default keymap with the trimmed tree
  and fails if it does not resolve.

Everything is compiled with `-Os -ffunction-sections -fdata-sections`, linked
with `-Wl,--gc-sections`, and stripped.

## Static linking

Both binaries are fully static; `manifest.json` reports `"static": true`. Alpine
ships static archives for zlib, libjpeg-turbo, pixman, libbsd, libX11 and
libxcb, and the Dockerfile builds the remaining ones (libXau, libXdmcp,
libfontenc, libXfont2, libxkbfile, libxcvt and libmd) from source because Alpine
has no `-static` subpackage for them.

Two details make the static link work:

- libtool treats `-static` as "prefer static libtool libraries", so `Xvnc` is
  relinked after the normal build with `-all-static`, which cannot be used
  during `configure` because it is not a compiler flag.
- libbsd's static archive expects the SHA1 symbols that live in libmd, so `-lmd`
  is added to the final link.

Because the binaries are static there is no loader, no `ld-musl-*.so.1` and no
`.so` closure to bundle, and no wrapper script is needed. Static linking was
verified end to end by unpacking the amd64 archive on an Ubuntu (glibc) host and
starting `bin/Xvnc` there: it served the `RFB 003.008` banner on a localhost
port and compiled the `pc+us+inet(evdev)` keymap from the bundled `share/xkb`
data without touching a single library from that host.

Keep it that way when bumping versions. If a future version cannot be linked
statically, the fallback is a dynamic musl binary shipped with
`lib/ld-musl-<arch>.so.1`, the exact `ldd` closure under `lib/`, and a
`bin/Xvnc` wrapper that execs the loader with `--library-path`; set
`"static": false` in `manifest.json` in that case. Treat it as a last resort and
repeat the glibc host test above before shipping it, since a wrapper that only
works inside the Alpine build image is worthless in a workspace.

## Publishing and pinning

The `desktop-runtime` workflow (`.github/workflows/desktop-runtime.yaml`) runs
when anything in this directory changes:

- On pull requests it builds both architectures, enforces `check_size.sh`, and
  runs `go test -tags desktop_runtime ./agent/x/agentdesktop/...` against the
  freshly built amd64 archive, which starts the real `Xvnc`.
- On `main` it additionally publishes the archives to
  `gs://releases.coder.com/desktop-runtime/<version>/` with `--no-clobber`, so
  published archives are immutable, and prints the `runtime.lock` values in the
  job summary.

Pinning is a separate, reviewable step: copy the printed `version` and sha256
values into `runtime.lock`. `make lint/desktop-runtime` fails when the pinned
version no longer matches `version.sh`, so a `Dockerfile` change cannot silently
ship an old archive. While `version` is empty (as it is before the first
publish), binaries build without the runtime and `desktopruntime.Available()`
reports false.

## Bumping TigerVNC

1. Update `TIGERVNC_VERSION` and `TIGERVNC_SHA256` in the `Dockerfile`.
2. Check the version of xorg-server that the release expects. TigerVNC's
   `BUILDING.txt` states the supported range and the `unix/xserver*.patch` files
   show which series it patches. Alpine's
   [tigervnc APKBUILD](https://gitlab.alpinelinux.org/alpine/aports/-/blob/master/community/tigervnc/APKBUILD)
   is a good reference for the exact xorg-server version and configure flags.
   Update `XORG_SERVER_VERSION` and `XORG_SERVER_SHA256` accordingly.
3. Open a PR; the workflow builds, size-checks and tests the result.
4. After merge, the workflow publishes the new version. Open a follow-up PR
   that updates `runtime.lock` with the values from the job summary.

The Alpine package versions used for the statically linked libraries are pinned
in `ARG APK_*_VERSION` and verified during the build. When a base image update
moves one of them, the build fails with the expected and actual version so that
the pin and the matching upstream license URL can be updated together.

## Measured sizes

Built from `alpine:3.22` with TigerVNC 1.16.0, xorg-server 21.1.22, xkbcomp
1.4.7 and xkeyboard-config 2.43:

| Target        | Compressed            | Unpacked              | `bin/Xvnc`  | `bin/xkbcomp` | `share/xkb` |
|---------------|-----------------------|-----------------------|-------------|---------------|-------------|
| `linux/amd64` | 1,793,051 B (1.7 MiB) | 4,689,003 B (4.5 MiB) | 2,761,256 B | 1,276,328 B   | 580 KiB     |
| `linux/arm64` | 1,737,086 B (1.7 MiB) | 4,752,763 B (4.5 MiB) | 2,772,888 B | 1,328,456 B   | 580 KiB     |
