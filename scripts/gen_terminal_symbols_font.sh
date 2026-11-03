#!/usr/bin/env bash

# Regenerates the "Terminal Symbols" fallback font used by the web
# terminal. Symbols Nerd Font Mono is split into one woff2 file per glyph set,
# and each file gets an @font-face rule with a matching unicode-range. Browsers
# only download a file once the terminal renders a glyph from its range, so a
# prompt that only uses Powerline separators never pays for the full icon set.
# It also writes the codepoints each file contains to a JSON manifest, which the
# Storybook glyph chart renders, and the licenses of every bundled icon set.
#
# Requires pyftsubset and ttx with brotli support:
#     pip install fonttools brotli

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

dependencies curl openssl pnpm pyftsubset tar ttx xz

NERD_FONTS_VERSION="v3.4.0"
NERD_FONTS_SHA256="7f8c090da3b0eaa7108646bf34cbbb6ed13d5358a72460522108b06c7ecd716a"
OUT_DIR="site/src/theme/fonts"
NERD_FONTS_RAW="https://raw.githubusercontent.com/ryanoasis/nerd-fonts/${NERD_FONTS_VERSION}"

# Each entry is "<project>|<license URL>", one per project whose glyphs the
# font bundles. Nerd Fonts keeps most icon set licenses next to its copies of
# the sets. The rest come from the upstream projects, pinned to a commit.
# license-audit.md in the Nerd Fonts repository lists the glyph sources.
licenses=(
	"Codicons|${NERD_FONTS_RAW}/src/glyphs/codicons/LICENSE.txt"
	"Devicons|https://raw.githubusercontent.com/vorillaz/devicons/8947499eeac633315de2a76fde08fc71619dd4ba/LICENSE"
	"Font Awesome|${NERD_FONTS_RAW}/src/glyphs/font-awesome/LICENSE.txt"
	"Font Awesome Extension|https://raw.githubusercontent.com/AndreLZGava/font-awesome-extension/09d80249058ee8018a45da30add4339289dbb466/LICENCE"
	"Font Logos|https://raw.githubusercontent.com/Lukas-W/font-logos/0b10f1ca42d323f27d0e41f2f38c695ff28f0eae/LICENSE"
	"IEC Power Symbols|https://raw.githubusercontent.com/jloughry/Unicode/d50497815991a1e44563a99b8fbd52913bb33d46/LICENSE.txt"
	"Material Design Icons|${NERD_FONTS_RAW}/src/glyphs/materialdesign/LICENSE"
	"Octicons|${NERD_FONTS_RAW}/src/glyphs/octicons/LICENSE"
	"Pomicons|${NERD_FONTS_RAW}/src/glyphs/pomicons/LICENSE"
	"Powerline Extra Symbols|${NERD_FONTS_RAW}/src/glyphs/powerline-extra/LICENSE"
	"Powerline Symbols|${NERD_FONTS_RAW}/src/glyphs/powerline-symbols/LICENSE.txt"
	"Seti UI|https://raw.githubusercontent.com/jesseweed/seti-ui/90214c965568022329405fc94d135a7419e544e2/LICENSE.md"
	"Weather Icons|${NERD_FONTS_RAW}/src/glyphs/weather-icons/OFL.txt"
)

# Each entry is "<set name>:<unicode ranges>". The ranges must not overlap and
# must cover every codepoint in the font except U+2665 (♥) and U+26A1 (⚡).
# This font sits ahead of the operating system's fallback fonts, so it would
# replace their glyphs for any character the terminal fonts lack. Those two
# are left to the system, which reliably has correct glyphs for them: ⚡ is
# a color emoji by default, and the Nerd Font ♥ is an outline heart rather
# than the filled heart suit.
glyph_sets=(
	"powerline:U+E0A0-E0A3,U+E0B0-E0D7"
	"misc:U+23FB-23FE,U+2630,U+276C-2771,U+2B58,U+E000-E00A"
	"font-awesome-extension:U+E200-E2A9"
	"weather:U+E300-E3E3"
	"seti:U+E5FA-E6B8"
	"devicons:U+E700-E8EF"
	"codicons:U+EA60-EC1E"
	"font-awesome:U+ED00-EFCE,U+F000-F2FF"
	"font-logos:U+F300-F381"
	"octicons:U+F400-F533"
	"material-design:U+F0001-F1AF0"
)

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

curl -fsSL -o "$tmp/symbols.tar.xz" \
	"https://github.com/ryanoasis/nerd-fonts/releases/download/${NERD_FONTS_VERSION}/NerdFontsSymbolsOnly.tar.xz"
# OpenSSL prints "SHA256(<file>)= <hash>" or "SHA2-256(<file>)= <hash>"
# depending on its version.
actual_sha256="$(openssl dgst -sha256 "$tmp/symbols.tar.xz" | awk '{ print $NF }')"
if [[ "$actual_sha256" != "$NERD_FONTS_SHA256" ]]; then
	error "NerdFontsSymbolsOnly.tar.xz has SHA-256 $actual_sha256, expected $NERD_FONTS_SHA256"
fi
xz -dc "$tmp/symbols.tar.xz" | tar -xf - -C "$tmp" SymbolsNerdFontMono-Regular.ttf LICENSE

rm -f "$OUT_DIR"/terminal-symbols-*.woff2

license_rule="$(printf '=%.0s' {1..80})"
# Prints a license section: a header naming the project and source, then the
# license text from stdin.
license_section() {
	printf '\n%s\n%s\n%s\n%s\n\n' "$license_rule" "$1" "$2" "$license_rule"
	awk '{ sub(/[[:space:]]+$/, ""); print }'
}
{
	cat <<EOF
Terminal Symbols is a subset of Symbols Nerd Font Mono ${NERD_FONTS_VERSION}, which
combines glyphs from the projects below. Nerd Fonts rescaled and repositioned
the glyphs, and scripts/gen_terminal_symbols_font.sh splits the font into
WOFF2 files by glyph set. Each project's license follows in full.

Glyph sources: https://github.com/ryanoasis/nerd-fonts/blob/${NERD_FONTS_VERSION}/license-audit.md
EOF
	license_section "Nerd Fonts" \
		"https://github.com/ryanoasis/nerd-fonts/releases/tag/${NERD_FONTS_VERSION}" \
		<"$tmp/LICENSE"
	for license in "${licenses[@]}"; do
		curl -fsSL "${license#*|}" | license_section "${license%%|*}" "${license#*|}"
	done
} >"$OUT_DIR/terminal-symbols-LICENSE.txt"

# Prints the codepoints a font file maps as a JSON array of inclusive
# [start, end] ranges.
codepoint_ranges() {
	ttx -q -t cmap -o - "$1" |
		grep -o 'code="0x[0-9a-f]*"' |
		cut -d'"' -f2 |
		while read -r hex; do echo "$((hex))"; done |
		sort -nu |
		awk '
			NR == 1 { printf "["; start = prev = $1; next }
			$1 == prev + 1 { prev = $1; next }
			{ printf "[%d,%d],", start, prev; start = prev = $1 }
			END { printf "[%d,%d]]", start, prev }
		'
}

manifest_entries=()

css="$OUT_DIR/terminal-symbols.css"
cat >"$css" <<EOF
/*
 * Code generated by scripts/gen_terminal_symbols_font.sh. DO NOT EDIT.
 *
 * Nerd Font glyphs subset from Symbols Nerd Font Mono ${NERD_FONTS_VERSION}.
 * Source: https://github.com/ryanoasis/nerd-fonts/releases/tag/${NERD_FONTS_VERSION}
 * Licenses: terminal-symbols-LICENSE.txt
 */
EOF

for glyph_set in "${glyph_sets[@]}"; do
	name="${glyph_set%%:*}"
	ranges="${glyph_set#*:}"
	file="terminal-symbols-${name}.woff2"

	# Keep hinting tables. Dropping them saves about 100 bytes per file, but
	# without the source font's cvt table FreeType (Chromium on Linux)
	# auto-hints the glyphs, so they render bolder and shifted compared to
	# the upstream font.
	pyftsubset "$tmp/SymbolsNerdFontMono-Regular.ttf" \
		--unicodes="$ranges" \
		--flavor=woff2 \
		--desubroutinize \
		--layout-features='' \
		--output-file="$OUT_DIR/$file"

	cat >>"$css" <<EOF

@font-face {
	font-family: "Terminal Symbols";
	font-style: normal;
	font-display: swap;
	font-weight: 400 700;
	src: url("./${file}?no-inline") format("woff2");
	unicode-range: ${ranges};
}
EOF

	manifest_entries+=("{\"name\":\"${name}\",\"ranges\":$(codepoint_ranges "$OUT_DIR/$file")}")
done

manifest="$OUT_DIR/terminal-symbols-glyphs.json"
(
	IFS=,
	echo "[${manifest_entries[*]}]"
) >"$manifest"

(cd site && pnpm exec biome format --write "../$css" "../$manifest")
