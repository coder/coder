# Minimal, self-contained Linux desktop runtime for the Coder workspace agent.
#
# The build stage compiles TigerVNC's Xvnc and xkbcomp against musl on Alpine
# and links both fully static, so the resulting tree runs on any Linux
# distribution regardless of its libc. The final stage is a scratch image that
# contains only the runtime tree, which lets `docker buildx build
# --output type=local` extract it directly.
#
# Size is the primary constraint, so anything that is not required to serve a
# localhost RFB session is disabled. See README.md for the rationale behind
# each exclusion.
#
# SOURCE_DATE_EPOCH is set so that the build timestamps GCC bakes into the
# binaries through __DATE__ and __TIME__, and the one CMake stamps into
# TigerVNC, do not change from build to build.

FROM alpine:3.22 AS builder

# Pinned upstream sources. Bump these together with the license URLs below.
ARG TIGERVNC_VERSION=1.16.0
ARG TIGERVNC_SHA256=10512fc0254ae3bde41c19d18c15f7ebd8cd476261afe0611c41965d635d46e8
ARG XORG_SERVER_VERSION=21.1.22
ARG XORG_SERVER_SHA256=1a242c8917c49ba29ccc1f6021613d8a2b9805dd0d271a66ae9d09f4b0bb06b3
ARG XKBCOMP_VERSION=1.4.7
ARG XKBCOMP_SHA256=0a288114e5f44e31987042c79aecff1ffad53a8154b8ec971c24a69a80f81f77
ARG LIBXAU_VERSION=1.0.12
ARG LIBXAU_SHA256=74d0e4dfa3d39ad8939e99bda37f5967aba528211076828464d2777d477fc0fb
ARG LIBXDMCP_VERSION=1.1.5
ARG LIBXDMCP_SHA256=d8a5222828c3adab70adf69a5583f1d32eb5ece04304f7f8392b6a353aa2228c
ARG LIBFONTENC_VERSION=1.1.8
ARG LIBFONTENC_SHA256=7b02c3d405236e0d86806b1de9d6868fe60c313628b38350b032914aa4fd14c6
ARG LIBXFONT2_VERSION=2.0.7
ARG LIBXFONT2_SHA256=8b7b82fdeba48769b69433e8e3fbb984a5f6bf368b0d5f47abeec49de3e58efb
ARG LIBXKBFILE_VERSION=1.1.3
ARG LIBXKBFILE_SHA256=a9b63eea997abb9ee6a8b4fbb515831c841f471af845a09de443b28003874bec
ARG LIBXCVT_VERSION=0.1.3
ARG LIBXCVT_SHA256=a929998a8767de7dfa36d6da4751cdbeef34ed630714f2f4a767b351f2442e01
ARG LIBMD_VERSION=1.1.0
ARG LIBMD_SHA256=1bd6aa42275313af3141c7cf2e5b964e8b1fd488025caf2f971f43b00776b332

# Versions of the Alpine packages whose static archives end up inside the
# binaries. They are verified at build time so that a distribution bump cannot
# silently ship a license text that does not match the linked code.
ARG APK_MUSL_VERSION=1.2.5
ARG APK_ZLIB_VERSION=1.3.2
ARG APK_LIBJPEG_TURBO_VERSION=3.1.0
ARG APK_PIXMAN_VERSION=0.46.4
ARG APK_LIBBSD_VERSION=0.12.2
ARG APK_LIBX11_VERSION=1.8.11
ARG APK_LIBXCB_VERSION=1.17.0
ARG APK_XKEYBOARD_CONFIG_VERSION=2.43
# GCC ships libstdc++ and libgcc, which are statically linked into Xvnc under
# the GCC Runtime Library Exception.
ARG GCC_VERSION=14.2.0

ENV CFLAGS="-Os -ffunction-sections -fdata-sections" \
	CXXFLAGS="-Os -ffunction-sections -fdata-sections" \
	LDFLAGS="-Wl,--gc-sections" \
	SOURCE_DATE_EPOCH=1577836800

# linux-pam-dev provides only the headers that the TigerVNC CMake build insists
# on; the PAM stub below replaces the library itself. No Mesa, GLX, GnuTLS,
# nettle, ffmpeg, systemd or wayland development packages are installed, which
# keeps the corresponding features out of the binaries by construction.
RUN apk add --no-cache \
	autoconf \
	automake \
	build-base \
	bzip2-static \
	cmake \
	curl \
	font-util-dev \
	libbsd-dev \
	libbsd-static \
	libjpeg-turbo-dev \
	libjpeg-turbo-static \
	libtool \
	libx11-dev \
	libx11-static \
	libxcb-dev \
	libxcb-static \
	linux-pam-dev \
	meson \
	ninja \
	patch \
	pixman-dev \
	pixman-static \
	pkgconf \
	python3 \
	samurai \
	util-macros \
	xkeyboard-config \
	xorgproto \
	xtrans \
	xz \
	zlib-dev \
	zlib-static

# Fail early if an Alpine package no longer matches the pinned license text.
RUN set -eux; \
	check() { \
		installed="$(apk list --installed "$1" 2>/dev/null | sed -n "s/^$1-\([0-9][^ ]*\)-r[0-9]* .*/\1/p" | head -n1)"; \
		if [ "$installed" != "$2" ]; then \
			echo "ERROR: $1 is $installed but $2 is pinned; update the ARG and the license URL" >&2; \
			exit 1; \
		fi; \
	}; \
	check musl "$APK_MUSL_VERSION"; \
	check zlib "$APK_ZLIB_VERSION"; \
	check libjpeg-turbo "$APK_LIBJPEG_TURBO_VERSION"; \
	check pixman "$APK_PIXMAN_VERSION"; \
	check libbsd "$APK_LIBBSD_VERSION"; \
	check libx11 "$APK_LIBX11_VERSION"; \
	check libxcb "$APK_LIBXCB_VERSION"; \
	check xkeyboard-config "$APK_XKEYBOARD_CONFIG_VERSION"

WORKDIR /build

# Download and verify every source tarball.
RUN set -eux; \
	fetch() { \
		curl -fsSL -o "/build/$(basename "$1")" "$1"; \
		echo "$2  /build/$(basename "$1")" | sha256sum -c -; \
	}; \
	x=https://www.x.org/releases/individual; \
	fetch "https://github.com/TigerVNC/tigervnc/archive/refs/tags/v${TIGERVNC_VERSION}.tar.gz" "$TIGERVNC_SHA256"; \
	fetch "https://www.x.org/releases/individual/xserver/xorg-server-${XORG_SERVER_VERSION}.tar.xz" "$XORG_SERVER_SHA256"; \
	fetch "$x/app/xkbcomp-${XKBCOMP_VERSION}.tar.xz" "$XKBCOMP_SHA256"; \
	fetch "$x/lib/libXau-${LIBXAU_VERSION}.tar.xz" "$LIBXAU_SHA256"; \
	fetch "$x/lib/libXdmcp-${LIBXDMCP_VERSION}.tar.xz" "$LIBXDMCP_SHA256"; \
	fetch "$x/lib/libfontenc-${LIBFONTENC_VERSION}.tar.xz" "$LIBFONTENC_SHA256"; \
	fetch "$x/lib/libXfont2-${LIBXFONT2_VERSION}.tar.xz" "$LIBXFONT2_SHA256"; \
	fetch "$x/lib/libxkbfile-${LIBXKBFILE_VERSION}.tar.xz" "$LIBXKBFILE_SHA256"; \
	fetch "$x/lib/libxcvt-${LIBXCVT_VERSION}.tar.xz" "$LIBXCVT_SHA256"; \
	fetch "https://archive.hadrons.org/software/libmd/libmd-${LIBMD_VERSION}.tar.xz" "$LIBMD_SHA256"; \
	for f in /build/*.tar.gz /build/*.tar.xz; do tar xf "$f"; done

# Alpine ships no static archives for these libraries, so build them here.
# libXfont2 drops the FreeType backend: the runtime ships no scalable fonts, so
# FreeType (plus libpng, brotli and bzip2) would only add weight. The built-in
# and PCF bitmap backends remain, which is what the X server needs to start.
RUN set -eux; \
	build() { \
		dir="$1"; shift; \
		cd "/build/$dir"; \
		./configure --prefix=/usr/local --disable-shared --enable-static "$@" >/dev/null; \
		make -j"$(nproc)" >/dev/null; \
		make install >/dev/null; \
	}; \
	build "libXau-${LIBXAU_VERSION}"; \
	build "libXdmcp-${LIBXDMCP_VERSION}"; \
	build "libfontenc-${LIBFONTENC_VERSION}"; \
	build "libXfont2-${LIBXFONT2_VERSION}" --disable-devel-docs --without-fop --disable-freetype --disable-bdfformat; \
	build "libxkbfile-${LIBXKBFILE_VERSION}"; \
	build "libmd-${LIBMD_VERSION}"

# libxcvt only knows how to build a shared library, and it is a single
# translation unit, so archive it by hand.
RUN set -eux; \
	cd "/build/libxcvt-${LIBXCVT_VERSION}"; \
	meson setup build --prefix=/usr/local --buildtype=minsize >/dev/null; \
	ninja -C build install >/dev/null; \
	rm -f /usr/local/lib/libxcvt.so*; \
	gcc $CFLAGS -Iinclude -c lib/libxcvt.c -o /tmp/libxcvt.o; \
	ar rcs /usr/local/lib/libxcvt.a /tmp/libxcvt.o; \
	sed -i 's|^Libs:.*|Libs: -L${libdir} -lxcvt|' /usr/local/lib/pkgconfig/libxcvt.pc

# Replace libpam with the stub and remove the shared library so that nothing
# can link against the real one by accident.
COPY pam_stub.c /build/pam_stub.c
RUN set -eux; \
	gcc $CFLAGS -c /build/pam_stub.c -o /tmp/pam_stub.o; \
	ar rcs /usr/local/lib/libpam.a /tmp/pam_stub.o; \
	rm -f /usr/lib/libpam.so* /usr/lib/libpam_misc.so* /usr/lib/libpamc.so*

# Build only the TigerVNC libraries that Xvnc links against. The viewer,
# x0vncserver and vncsession are not part of the runtime.
RUN set -eux; \
	cd "/build/tigervnc-${TIGERVNC_VERSION}"; \
	cmake -B . -G Ninja \
		-DCMAKE_INSTALL_PREFIX=/usr \
		-DCMAKE_BUILD_TYPE=MinSizeRel \
		-DBUILD_SHARED_LIBS=OFF \
		-DBUILD_VIEWER=OFF \
		-DENABLE_GNUTLS=OFF \
		-DENABLE_NETTLE=OFF \
		-DENABLE_H264=OFF \
		-DENABLE_NLS=OFF \
		-DENABLE_PWQUALITY=OFF \
		-DENABLE_SELINUX=OFF \
		-DENABLE_SYSTEMD=OFF \
		-DENABLE_WAYLAND=OFF \
		-DINSTALL_SYSTEMD_UNITS=OFF >/dev/null; \
	cmake --build . --target unixcommon rfb network rdr core >/dev/null

# TigerVNC builds Xvnc inside an xorg-server source tree (see its BUILDING.txt
# and Alpine's community/tigervnc APKBUILD).
#
# The runtime is unpacked to an arbitrary directory in the workspace, so no
# build-time absolute path may be required at run time:
#   - An empty xkb bin directory makes the server look up xkbcomp on PATH, so
#     the launcher only has to prepend the runtime's bin directory. Without it
#     the compiled-in path would point at a directory that does not exist in the
#     workspace and keyboard initialization would be fatal.
#   - The xkb data directory is compiled in but always overridden with -xkbdir.
#   - The default font path is the libXfont2 built-in fonts, which live inside
#     the binary, so Xvnc starts with no font directory present and without the
#     launcher having to pass -fp.
#   - The compiled keymap is written to /tmp, which keeps the unpacked runtime
#     directory itself read-only friendly.
RUN set -eux; \
	cd "/build/tigervnc-${TIGERVNC_VERSION}"; \
	cp -r "/build/xorg-server-${XORG_SERVER_VERSION}/." unix/xserver/; \
	cd unix/xserver; \
	patch -p1 <../xserver21.patch >/dev/null; \
	autoreconf -fi >/dev/null 2>&1; \
	./configure \
		--prefix=/usr \
		--disable-config-hal \
		--disable-config-udev \
		--disable-dmx \
		--disable-dri \
		--disable-dri2 \
		--disable-dri3 \
		--disable-glamor \
		--disable-glx \
		--disable-kdrive \
		--disable-libdrm \
		--disable-shared \
		--enable-static \
		--disable-devel-docs \
		--disable-docs \
		--disable-selective-werror \
		--disable-unit-tests \
		--disable-xephyr \
		--disable-xinerama \
		--disable-xnest \
		--disable-xorg \
		--disable-xvfb \
		--disable-xwayland \
		--disable-xwin \
		--without-dtrace \
		--with-default-font-path=built-ins \
		--with-xkb-bin-directory= \
		--with-xkb-output=/tmp \
		--with-xkb-path=/usr/share/X11/xkb >/dev/null; \
	make -j"$(nproc)" TIGERVNC_SRCDIR="/build/tigervnc-${TIGERVNC_VERSION}" >/dev/null 2>&1

# Relink Xvnc fully static. libtool needs -all-static, which cannot be used
# during configure because it is not a compiler flag. -lmd resolves the SHA1
# symbols that libbsd's static archive expects.
RUN set -eux; \
	cd "/build/tigervnc-${TIGERVNC_VERSION}/unix/xserver"; \
	rm -f hw/vnc/Xvnc; \
	make -C hw/vnc Xvnc \
		TIGERVNC_SRCDIR="/build/tigervnc-${TIGERVNC_VERSION}" \
		LIBS="-lm -lbsd -lmd" \
		LDFLAGS="-all-static -Wl,--gc-sections" >/dev/null 2>&1; \
	strip hw/vnc/Xvnc; \
	file hw/vnc/Xvnc | grep -q "statically linked" || { \
		echo "ERROR: Xvnc is not statically linked" >&2; \
		exit 1; \
	}

RUN set -eux; \
	cd "/build/xkbcomp-${XKBCOMP_VERSION}"; \
	LDFLAGS="-static -Wl,--gc-sections" ./configure --prefix=/usr/local >/dev/null; \
	make -j"$(nproc)" LIBS="-lxkbfile -lX11 -lxcb -lXau -lXdmcp" >/dev/null; \
	strip xkbcomp; \
	file xkbcomp | grep -q "statically linked" || { \
		echo "ERROR: xkbcomp is not statically linked" >&2; \
		exit 1; \
	}

# Assemble the runtime tree.
RUN set -eux; \
	mkdir -p /runtime/bin /runtime/LICENSES; \
	cp "/build/tigervnc-${TIGERVNC_VERSION}/unix/xserver/hw/vnc/Xvnc" /runtime/bin/Xvnc; \
	cp "/build/xkbcomp-${XKBCOMP_VERSION}/xkbcomp" /runtime/bin/xkbcomp

# Trim xkeyboard-config down to what a us/pc105/evdev keymap needs. The XML and
# LST catalogues only feed configuration GUIs, and every layout other than us
# is dead weight. types and compat are small and heavily cross-included, so
# they are copied whole.
RUN set -eux; \
	src=/usr/share/X11/xkb; \
	dst=/runtime/share/xkb; \
	mkdir -p "$dst/rules" "$dst/keycodes" "$dst/types" "$dst/compat" "$dst/symbols" "$dst/geometry"; \
	cp "$src/rules/evdev" "$dst/rules/evdev"; \
	cp "$src/keycodes/evdev" "$src/keycodes/aliases" "$dst/keycodes/"; \
	find "$src/types" -maxdepth 1 -type f -exec cp {} "$dst/types/" \;; \
	find "$src/compat" -maxdepth 1 -type f -exec cp {} "$dst/compat/" \;; \
	for s in us pc inet altwin srvr_ctrl keypad level3 level5 capslock ctrl compose shift nbsp terminate eurosign rupeesign group; do \
		cp "$src/symbols/$s" "$dst/symbols/$s"; \
	done; \
	cp "$src/geometry/pc" "$dst/geometry/pc"

# Fail the build if the trimmed data cannot produce the default keymap.
RUN set -eux; \
	printf '%s\n' \
		'xkb_keymap {' \
		'  xkb_keycodes { include "evdev+aliases(qwerty)" };' \
		'  xkb_types { include "complete" };' \
		'  xkb_compat { include "complete" };' \
		'  xkb_symbols { include "pc+us+inet(evdev)" };' \
		'  xkb_geometry { include "pc(pc105)" };' \
		'};' >/tmp/keymap.xkb; \
	/runtime/bin/xkbcomp -I -I/runtime/share/xkb -xkm /tmp/keymap.xkb /tmp/keymap.xkm >/tmp/xkbcomp.log 2>&1; \
	if grep -q "^Error" /tmp/xkbcomp.log || [ ! -s /tmp/keymap.xkm ]; then \
		cat /tmp/xkbcomp.log >&2; \
		echo "ERROR: the trimmed xkb data does not compile" >&2; \
		exit 1; \
	fi

# Collect the license of every bundled component. Sources we build ship their
# own license file; for Alpine packages the text comes from the upstream tag
# that matches the pinned package version.
RUN set -eux; \
	l=/runtime/LICENSES; \
	cp "/build/tigervnc-${TIGERVNC_VERSION}/LICENCE.TXT" "$l/tigervnc.txt"; \
	cp "/build/xorg-server-${XORG_SERVER_VERSION}/COPYING" "$l/xorg-server.txt"; \
	cp "/build/xkbcomp-${XKBCOMP_VERSION}/COPYING" "$l/xkbcomp.txt"; \
	cp "/build/libXau-${LIBXAU_VERSION}/COPYING" "$l/libXau.txt"; \
	cp "/build/libXdmcp-${LIBXDMCP_VERSION}/COPYING" "$l/libXdmcp.txt"; \
	cp "/build/libfontenc-${LIBFONTENC_VERSION}/COPYING" "$l/libfontenc.txt"; \
	cp "/build/libXfont2-${LIBXFONT2_VERSION}/COPYING" "$l/libXfont2.txt"; \
	cp "/build/libxkbfile-${LIBXKBFILE_VERSION}/COPYING" "$l/libxkbfile.txt"; \
	cp "/build/libxcvt-${LIBXCVT_VERSION}/COPYING" "$l/libxcvt.txt"; \
	cp "/build/libmd-${LIBMD_VERSION}/COPYING" "$l/libmd.txt"; \
	curl -fsSL -o "$l/xkeyboard-config.txt" "https://gitlab.freedesktop.org/xkeyboard-config/xkeyboard-config/-/raw/xkeyboard-config-${APK_XKEYBOARD_CONFIG_VERSION}/COPYING"; \
	curl -fsSL -o "$l/zlib.txt" "https://raw.githubusercontent.com/madler/zlib/v1.3.1/LICENSE"; \
	curl -fsSL -o "$l/libjpeg-turbo.txt" "https://raw.githubusercontent.com/libjpeg-turbo/libjpeg-turbo/${APK_LIBJPEG_TURBO_VERSION}/LICENSE.md"; \
	curl -fsSL -o "$l/pixman.txt" "https://gitlab.freedesktop.org/pixman/pixman/-/raw/pixman-${APK_PIXMAN_VERSION}/COPYING"; \
	curl -fsSL -o "$l/libbsd.txt" "https://gitlab.freedesktop.org/libbsd/libbsd/-/raw/${APK_LIBBSD_VERSION}/COPYING"; \
	curl -fsSL -o "$l/libX11.txt" "https://gitlab.freedesktop.org/xorg/lib/libx11/-/raw/libX11-${APK_LIBX11_VERSION}/COPYING"; \
	curl -fsSL -o "$l/libxcb.txt" "https://gitlab.freedesktop.org/xorg/lib/libxcb/-/raw/libxcb-${APK_LIBXCB_VERSION}/COPYING"; \
	curl -fsSL -o "$l/musl.txt" "https://git.musl-libc.org/cgit/musl/plain/COPYRIGHT?h=v${APK_MUSL_VERSION}"; \
	curl -fsSL -o "$l/gcc-runtime-exception.txt" "https://raw.githubusercontent.com/gcc-mirror/gcc/releases/gcc-${GCC_VERSION}/COPYING.RUNTIME"; \
	for f in "$l"/*; do [ -s "$f" ] || { echo "ERROR: empty license file $f" >&2; exit 1; }; done

# zlib 1.3.1 is the newest upstream tag; Alpine's 1.3.2 is a fork-free rebuild
# and carries the same license text.
ARG TARGETARCH
RUN set -eux; \
	printf '%s\n' \
		'{' \
		'  "schema_version": 1,' \
		"  \"arch\": \"${TARGETARCH}\"," \
		'  "components": [' \
		"    {\"name\": \"tigervnc\", \"version\": \"${TIGERVNC_VERSION}\"}," \
		"    {\"name\": \"xorg-server\", \"version\": \"${XORG_SERVER_VERSION}\"}," \
		"    {\"name\": \"xkbcomp\", \"version\": \"${XKBCOMP_VERSION}\"}," \
		"    {\"name\": \"xkeyboard-config\", \"version\": \"${APK_XKEYBOARD_CONFIG_VERSION}\"}" \
		'  ],' \
		'  "static": true' \
		'}' >/runtime/manifest.json; \
	case "$TARGETARCH" in \
	amd64 | arm64) ;; \
	*) echo "ERROR: unsupported architecture '$TARGETARCH'" >&2; exit 1 ;; \
	esac

# Normalize timestamps so that the exported tree hashes identically across
# builds.
RUN find /runtime -exec touch -h -d @0 {} +

FROM scratch
COPY --from=builder /runtime/ /
