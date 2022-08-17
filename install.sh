#!/bin/sh
set -eu

# Coder's automatic install script.
# See https://github.com/coder/coder#installing-coder

usage() {
	arg0="$0"
	if [ "$0" = sh ]; then
		arg0="curl -fsSL https://coder.com/install.sh | sh -s --"
	else
		not_curl_usage="The latest script is available at https://coder.com/install.sh
"
	fi

	cath <<EOF
Installs Coder.
It tries to use the system package manager if possible.
After successful installation it explains how to start Coder.

Pass in user@host to install Coder on user@host over ssh.
The remote host must have internet access.
${not_curl_usage-}
Usage:

  $arg0 [--dry-run] [--version X.X.X] [--edge] [--method detect] \
        [--prefix ~/.local] [--rsh ssh] [user@host]

  --dry-run
      Echo the commands for the install process without running them.

  --version X.X.X
      Install a specific version instead of the latest.

  --edge
      Install the latest edge version instead of the latest stable version.

  --method [detect | standalone]
      Choose the installation method. Defaults to detect.
      - detect detects the system package manager and tries to use it.
        Full reference on the process is further below.
      - standalone installs a standalone release archive into /usr/local/bin

  --prefix <dir>
      Sets the prefix used by standalone release archives. Defaults to /usr/local
      and the binary is copied into /usr/local/bin
      To install in \$HOME, pass ---prefix=\$HOME/.local
	
  --binary-name <name>
	  Sets the name for the CLI in standalone release archives. Defaults to "coder"
	  To use the CLI as coder2, pass --binary-name=coder2
	  Note: in-product documentation will always refer to the CLI as "coder"

  --rsh <bin>
      Specifies the remote shell for remote installation. Defaults to ssh.

The detection method works as follows:
  - Debian, Ubuntu, Raspbian: install the deb package from GitHub.
  - Fedora, CentOS, RHEL, openSUSE: install the rpm package from GitHub.
  - Alpine: install the apk package from GitHub.
  - macOS: install the release from GitHub.
  - All others: install the release from GitHub.

We build releases on GitHub for amd64, armv7, and arm64 on Windows, Linux, and macOS.

When the detection method tries to pull a release from GitHub it will
fall back to installing standalone when there is no matching release for
the system's operating system and architecture.

The installer will cache all downloaded assets into ~/.cache/coder
EOF
}

echo_latest_version() {
	if [ "${EDGE-}" ]; then
		version="$(curl -fsSL https://api.github.com/repos/coder/coder/releases | awk 'match($0,/.*"html_url": "(.*\/releases\/tag\/.*)".*/)' | head -n 1 | awk -F '"' '{print $4}')"
	else
		# https://gist.github.com/lukechilds/a83e1d7127b78fef38c2914c4ececc3c#gistcomment-2758860
		version="$(curl -fsSLI -o /dev/null -w "%{url_effective}" https://github.com/coder/coder/releases/latest)"
	fi
	version="${version#https://github.com/coder/coder/releases/tag/}"
	version="${version#v}"
	echo "$version"
}

echo_standalone_postinstall() {
	if [ "${DRY_RUN-}" ]; then
		echo_dryrun_postinstall
		return
	fi

	cath <<EOF

Standalone release has been installed into $STANDALONE_INSTALL_PREFIX/bin/$STANDALONE_BINARY_NAME

EOF

	if [ "$STANDALONE_INSTALL_PREFIX" != /usr/local ]; then
		cath <<EOF
Extend your path to use Coder:
PATH="$STANDALONE_INSTALL_PREFIX/bin:\$PATH"

EOF
	fi
	cath <<EOF
Run Coder:
  $STANDALONE_BINARY_NAME server

EOF
}

echo_systemd_postinstall() {
	if [ "${DRY_RUN-}" ]; then
		echo_dryrun_postinstall
		return
	fi

	echoh
	cath <<EOF
$1 package has been installed.

To run Coder as a system service:

  # Set up an external access URL or enable CODER_TUNNEL
  $ sudo vim /etc/coder.d/coder.env
  # Use systemd to start Coder now and on reboot
  $ sudo systemctl enable --now coder
  # View the logs to ensure a successful start
  $ journalctl -u coder.service -b

Or, just run the server directly:

  $ coder server

EOF
}

echo_dryrun_postinstall() {
	cath <<EOF

Dry-run complete.

To install Coder, re-run this script without the --dry-run flag.
EOF
}

main() {
	if [ "${TRACE-}" ]; then
		set -x
	fi

	unset \
		DRY_RUN \
		METHOD \
		OPTIONAL \
		ALL_FLAGS \
		RSH_ARGS \
		EDGE \
		RSH

	ALL_FLAGS=""
	while [ "$#" -gt 0 ]; do
		case "$1" in
		-*)
			ALL_FLAGS="${ALL_FLAGS} $1"
			;;
		esac

		case "$1" in
		--dry-run)
			DRY_RUN=1
			;;
		--method)
			METHOD="$(parse_arg "$@")"
			shift
			;;
		--method=*)
			METHOD="$(parse_arg "$@")"
			;;
		--prefix)
			STANDALONE_INSTALL_PREFIX="$(parse_arg "$@")"
			shift
			;;
		--prefix=*)
			STANDALONE_INSTALL_PREFIX="$(parse_arg "$@")"
			;;
		--binary-name)
			STANDALONE_BINARY_NAME="$(parse_arg "$@")"
			shift
			;;
		--binary-name=*)
			STANDALONE_BINARY_NAME="$(parse_arg "$@")"
			;;
		--version)
			VERSION="$(parse_arg "$@")"
			shift
			;;
		--version=*)
			VERSION="$(parse_arg "$@")"
			;;
		--edge)
			EDGE=1
			;;
		--rsh)
			RSH="$(parse_arg "$@")"
			shift
			;;
		--rsh=*)
			RSH="$(parse_arg "$@")"
			;;
		-h | --h | -help | --help)
			usage
			exit 0
			;;
		--)
			shift
			# We remove the -- added above.
			ALL_FLAGS="${ALL_FLAGS% --}"
			RSH_ARGS="$*"
			break
			;;
		-*)
			echoerr "Unknown flag $1"
			echoerr "Run with --help to see usage."
			exit 1
			;;
		*)
			RSH_ARGS="$*"
			break
			;;
		esac

		shift
	done

	if [ "${RSH_ARGS-}" ]; then
		RSH="${RSH-ssh}"
		echoh "Installing remotely with $RSH $RSH_ARGS"
		curl -fsSL https://coder.dev/install.sh | prefix "$RSH_ARGS" "$RSH" "$RSH_ARGS" sh -s -- "$ALL_FLAGS"
		return
	fi

	METHOD="${METHOD-detect}"
	if [ "$METHOD" != detect ] && [ "$METHOD" != standalone ]; then
		echoerr "Unknown install method \"$METHOD\""
		echoerr "Run with --help to see usage."
		exit 1
	fi

	# These are used by the various install_* functions that make use of GitHub
	# releases in order to download and unpack the right release.
	CACHE_DIR=$(echo_cache_dir)
	STANDALONE_INSTALL_PREFIX=${STANDALONE_INSTALL_PREFIX:-/usr/local}
	STANDALONE_BINARY_NAME=${STANDALONE_BINARY_NAME:-coder}
	VERSION=${VERSION:-$(echo_latest_version)}
	# These can be overridden for testing but shouldn't normally be used as it can
	# result in a broken coder.
	OS=${OS:-$(os)}
	ARCH=${ARCH:-$(arch)}

	distro_name

	if [ "${DRY_RUN-}" ]; then
		echoh "Running with --dry-run; the following are the commands that would be run if this were a real installation:"
		echoh
	fi

	# Standalone installs by pulling pre-built releases from GitHub.
	if [ "$METHOD" = standalone ]; then
		if has_standalone; then
			install_standalone
			exit 0
		else
			echoerr "There are no standalone releases for $ARCH"
			echoerr "Please try again without '--method standalone'"
			exit 1
		fi
	fi

	# DISTRO can be overridden for testing but shouldn't normally be used as it
	# can result in a broken coder.
	DISTRO=${DISTRO:-$(distro)}

	case $DISTRO in
	# macOS uses the standalone installation for now.
	# Homebrew support is planned. See: https://github.com/coder/coder/issues/1925
	macos) install_standalone ;;
	# The .deb and .rpm files are pulled from GitHub.
	debian) install_deb ;;
	fedora | opensuse) install_rpm ;;
	# We don't have GitHub releases that work on Alpine or FreeBSD so we have no
	# choice but to use npm here.
	alpine) install_apk ;;
	# For anything else we'll try to install standalone but fall back to npm if
	# we don't have releases for the architecture.
	*)
		echoh "Unsupported package manager."
		echoh "Falling back to standalone installation."
		install_standalone
		;;
	esac
}

parse_arg() {
	case "$1" in
	*=*)
		# Remove everything after first equal sign.
		opt="${1%%=*}"
		# Remove everything before first equal sign.
		optarg="${1#*=}"
		if [ ! "$optarg" ] && [ ! "${OPTIONAL-}" ]; then
			echoerr "$opt requires an argument"
			echoerr "Run with --help to see usage."
			exit 1
		fi
		echo "$optarg"
		return
		;;
	esac

	case "${2-}" in
	"" | -*)
		if [ ! "${OPTIONAL-}" ]; then
			echoerr "$1 requires an argument"
			echoerr "Run with --help to see usage."
			exit 1
		fi
		;;
	*)
		echo "$2"
		return
		;;
	esac
}

fetch() {
	URL="$1"
	FILE="$2"

	if [ -e "$FILE" ]; then
		echoh "+ Reusing $FILE"
		return
	fi

	sh_c mkdir -p "$CACHE_DIR"
	sh_c curl \
		-#fL \
		-o "$FILE.incomplete" \
		-C - \
		"$URL"
	sh_c mv "$FILE.incomplete" "$FILE"
}

install_deb() {
	echoh "Installing v$VERSION of the $ARCH deb package from GitHub."
	echoh

	fetch "https://github.com/coder/coder/releases/download/v$VERSION/coder_${VERSION}_${OS}_${ARCH}.deb" \
		"$CACHE_DIR/coder_${VERSION}_$ARCH.deb"
	sudo_sh_c dpkg --force-confdef --force-confold -i "$CACHE_DIR/coder_${VERSION}_$ARCH.deb"

	echo_systemd_postinstall deb
}

install_rpm() {
	echoh "Installing v$VERSION of the $ARCH rpm package from GitHub."
	echoh

	fetch "https://github.com/coder/coder/releases/download/v$VERSION/coder_${VERSION}_${OS}_${ARCH}.rpm" \
		"$CACHE_DIR/coder_${VERSION}_${OS}_${ARCH}.rpm"
	sudo_sh_c rpm -i "$CACHE_DIR/coder_${VERSION}_${OS}_${ARCH}.rpm"

	echo_systemd_postinstall rpm
}

install_apk() {
	echoh "Installing v$VERSION of the $ARCH apk package from GitHub."
	echoh

	fetch "https://github.com/coder/coder/releases/download/v$VERSION/coder_${VERSION}_${OS}_${ARCH}.apk" \
		"$CACHE_DIR/coder_${VERSION}_${OS}_${ARCH}.apk"
	sudo_sh_c apk add --allow-untrusted "$CACHE_DIR/coder_${VERSION}_${OS}_${ARCH}.apk"

	echo_systemd_postinstall apk
}

install_standalone() {
	echoh "Installing v$VERSION of the $ARCH release from GitHub."
	echoh

	# macOS releases are packaged as .zip
	case $OS in
	darwin) STANDALONE_ARCHIVE_FORMAT=zip ;;
	*) STANDALONE_ARCHIVE_FORMAT=tar.gz ;;
	esac

	fetch "https://github.com/coder/coder/releases/download/v$VERSION/coder_${VERSION}_${OS}_${ARCH}.$STANDALONE_ARCHIVE_FORMAT" \
		"$CACHE_DIR/coder_${VERSION}_${OS}_${ARCH}.$STANDALONE_ARCHIVE_FORMAT"

	# -w only works if the directory exists so try creating it first. If this
	# fails we can ignore the error as the -w check will then swap us to sudo.
	sh_c mkdir -p "$STANDALONE_INSTALL_PREFIX" 2>/dev/null || true

	sh_c="sh_c"
	if [ ! -w "$STANDALONE_INSTALL_PREFIX" ]; then
		sh_c="sudo_sh_c"
	fi

	"$sh_c" mkdir -p "$STANDALONE_INSTALL_PREFIX/bin"
	if [ "$STANDALONE_ARCHIVE_FORMAT" = tar.gz ]; then
		"$sh_c" tar -C "$CACHE_DIR" -xzf "$CACHE_DIR/coder_${VERSION}_${OS}_${ARCH}.tar.gz"
	else
		"$sh_c" unzip -d "$CACHE_DIR" -o "$CACHE_DIR/coder_${VERSION}_${OS}_${ARCH}.zip"
	fi

	COPY_LOCATION="$STANDALONE_INSTALL_PREFIX/bin/$STANDALONE_BINARY_NAME"

	# Remove the file if it already exists to
	# avoid https://github.com/coder/coder/issues/2086
	if [ -f "$COPY_LOCATION" ]; then
		"$sh_c" rm "$COPY_LOCATION"
	fi

	# Copy the binary to the correct location.
	"$sh_c" cp "$CACHE_DIR/coder" "$COPY_LOCATION"

	echo_standalone_postinstall
}

# Determine if we have standalone releases on GitHub for the system's arch.
has_standalone() {
	case $ARCH in
	amd64) return 0 ;;
	ard64) return 0 ;;
	armv7)
		[ "$(distro)" != darwin ]
		return
		;;
	*) return 1 ;;
	esac
}

os() {
	uname="$(uname)"
	case $uname in
	Linux) echo linux ;;
	Darwin) echo darwin ;;
	FreeBSD) echo freebsd ;;
	*) echo "$uname" ;;
	esac
}

# Print the detected Linux distro, otherwise print the OS name.
#
# Example outputs:
# - darwin -> darwin
# - freebsd -> freebsd
# - ubuntu, raspbian, debian ... -> debian
# - amzn, centos, rhel, fedora, ... -> fedora
# - opensuse-{leap,tumbleweed} -> opensuse
# - alpine -> alpine
# - arch -> arch
#
# Inspired by https://github.com/docker/docker-install/blob/26ff363bcf3b3f5a00498ac43694bf1c7d9ce16c/install.sh#L111-L120.
distro() {
	if [ "$OS" = "darwin" ] || [ "$OS" = "freebsd" ]; then
		echo "$OS"
		return
	fi

	if [ -f /etc/os-release ]; then
		(
			. /etc/os-release
			if [ "${ID_LIKE-}" ]; then
				for id_like in $ID_LIKE; do
					case "$id_like" in debian | fedora | opensuse)
						echo "$id_like"
						return
						;;
					esac
				done
			fi

			echo "$ID"
		)
		return
	fi
}

# Print a human-readable name for the OS/distro.
distro_name() {
	if [ "$(uname)" = "Darwin" ]; then
		echo "macOS v$(sw_vers -productVersion)"
		return
	fi

	if [ -f /etc/os-release ]; then
		(
			. /etc/os-release
			echo "$PRETTY_NAME"
		)
		return
	fi

	# Prints something like: Linux 4.19.0-9-amd64
	uname -sr
}

arch() {
	uname_m=$(uname -m)
	case $uname_m in
	aarch64) echo arm64 ;;
	x86_64) echo amd64 ;;
	armv7l) echo armv7 ;;
	*) echo "$uname_m" ;;
	esac
}

command_exists() {
	if [ ! "$1" ]; then return 1; fi
	command -v "$@" >/dev/null
}

sh_c() {
	echoh "+ $*"
	if [ ! "${DRY_RUN-}" ]; then
		sh -c "$*"
	fi
}

sudo_sh_c() {
	if [ "$(id -u)" = 0 ]; then
		sh_c "$@"
	elif command_exists sudo; then
		sh_c "sudo $*"
	elif command_exists su; then
		sh_c "su - -c '$*'"
	else
		echoh
		echoerr "This script needs to run the following command as root."
		echoerr "  $*"
		echoerr "Please install sudo or su."
		exit 1
	fi
}

echo_cache_dir() {
	if [ "${XDG_CACHE_HOME-}" ]; then
		echo "$XDG_CACHE_HOME/coder"
	elif [ "${HOME-}" ]; then
		echo "$HOME/.cache/coder"
	else
		echo "/tmp/coder-cache"
	fi
}

echoh() {
	echo "$@" | humanpath
}

cath() {
	humanpath
}

echoerr() {
	echoh "$@" >&2
}

# humanpath replaces all occurrences of " $HOME" with " ~"
# and all occurrences of '"$HOME' with the literal '"$HOME'.
humanpath() {
	sed "s# $HOME# ~#g; s#\"$HOME#\"\$HOME#g"
}

# We need to make sure we exit with a non zero exit if the command fails.
# /bin/sh does not support -o pipefail unfortunately.
prefix() {
	PREFIX="$1"
	shift
	fifo="$(mktemp -d)/fifo"
	mkfifo "$fifo"
	sed -e "s#^#$PREFIX: #" "$fifo" &
	"$@" >"$fifo" 2>&1
}

main "$@"
