/**
 * Emoji asset pipeline.
 *
 * `static/emojis/*.png` are public URLs (`/emojis/1f4bb.png`) referenced by
 * templates, proxies and agent log sources, so the file set is an API surface.
 * They are synchronized from the pinned `emoji-datasource-apple` devDependency
 * together with a normalized runtime manifest.
 *
 * `static/emojis/spritesheet.png` is legacy and unmanaged. Nothing in the app
 * reads it now that the picker renders individual images, but
 * `/emojis/spritesheet.png` is a public URL, so it stays committed and this
 * script neither regenerates nor deletes it.
 *
 * The datasource carries legacy Unicode names ("HEAVY BLACK HEART") and no
 * synonyms, which makes it a poor search corpus. Each record is therefore
 * enriched from two pinned metadata packages keyed by emoji glyph:
 * `unicode-emoji-json` supplies the canonical English `name` ("red heart")
 * that replaces the legacy one, and `emojilib` supplies `keywords` ("love",
 * "like", "valentines"). The glyph itself is only the join key between the
 * datasource and those packages, so it is not emitted.
 *
 * Usage:
 *   node scripts/update-emojis.mjs           # sync images and manifest
 *   node scripts/update-emojis.mjs --check   # verify committed artifacts
 */
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { copyFileSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

// Vitest rewrites `new URL("..", import.meta.url)` to its dev server origin,
// so derive the site directory from the file path instead.
const scriptPath = fileURLToPath(import.meta.url);
const siteDir = dirname(dirname(scriptPath));

const require = createRequire(scriptPath);
const PACKAGE_DIR = dirname(require.resolve("emoji-datasource-apple"));
const SOURCE_IMAGES_DIR = join(PACKAGE_DIR, "img/apple/64");
export const IMAGES_DIR = join(siteDir, "static/emojis");
const LEGACY_SPRITESHEET = "spritesheet.png";
const MANIFEST_NAME = "emojiDataGenerated.json";
const MANIFEST_RELATIVE = `src/components/IconField/${MANIFEST_NAME}`;
const MANIFEST_PATH = join(siteDir, MANIFEST_RELATIVE);

// Skin tone modifiers are not selectable emoji.
const EXCLUDED_CATEGORY = "Component";

// Metadata packages, both keyed by the rendered emoji glyph.
const NAMES_PACKAGE = "unicode-emoji-json";
const KEYWORDS_PACKAGE = "emojilib";
const CANONICAL_NAMES = require("unicode-emoji-json/data-by-emoji.json");
const SEARCH_KEYWORDS = require("emojilib");

// Codepoints are hyphen-separated hex, and Unicode tops out at U+10FFFF.
const CODEPOINT_PATTERN = /^[0-9a-f]{1,6}$/i;
const MAX_CODEPOINT = 0x10ffff;

// The datasource ships untyped JSON, so assert the fields we depend on. The
// datasource `name` is not among them: it is replaced by the canonical one.
const RECORD_STRING_FIELDS = [
	"short_name",
	"unified",
	"image",
	"category",
	"subcategory",
];
const RECORD_FIELDS = [...RECORD_STRING_FIELDS, "sort_order", "has_img_apple"];
const SKIN_STRING_FIELDS = ["unified", "image"];
const SKIN_FIELDS = [...SKIN_STRING_FIELDS, "has_img_apple"];

function requireObject(value, label) {
	if (value === null || typeof value !== "object" || Array.isArray(value)) {
		throw new Error(`${label} is not an object`);
	}
}

function requireFields(value, fields, label) {
	requireObject(value, label);
	for (const field of fields) {
		if (value[field] === undefined || value[field] === null) {
			throw new Error(`${label} is missing "${field}"`);
		}
	}
}

function requireStringFields(value, fields, label) {
	for (const field of fields) {
		if (typeof value[field] !== "string") {
			throw new Error(`${label} has a non-string "${field}"`);
		}
	}
}

function requireStringArray(value, label) {
	if (!Array.isArray(value) || value.some((item) => typeof item !== "string")) {
		throw new Error(`${label} is not an array of strings`);
	}
}

function recordLabel(record, index) {
	return typeof record?.short_name === "string" && record.short_name
		? `emoji "${record.short_name}"`
		: `record ${index}`;
}

/**
 * Render a hyphenated codepoint sequence as its emoji glyph. This is only the
 * join key against the metadata packages, which are keyed by glyph; it is not
 * part of the manifest.
 */
export function emojiGlyph(unified, label) {
	const codepoints = unified.split("-").map((segment) => {
		if (!CODEPOINT_PATTERN.test(segment)) {
			throw new Error(`${label} has a non-hexadecimal codepoint "${segment}"`);
		}
		const codepoint = Number.parseInt(segment, 16);
		if (codepoint > MAX_CODEPOINT) {
			throw new Error(`${label} has an out-of-range codepoint "${segment}"`);
		}
		return codepoint;
	});
	return String.fromCodePoint(...codepoints);
}

/**
 * Resolve the canonical English name and search keywords for a glyph. A glyph
 * the metadata packages do not know about is a hard failure: silently shipping
 * an emoji with a legacy name and no synonyms is the regression this pipeline
 * exists to prevent. The sources are injectable so the failure modes stay
 * testable without a fixture package.
 */
export function emojiMetadata(
	glyph,
	label,
	names = CANONICAL_NAMES,
	keywords = SEARCH_KEYWORDS,
) {
	if (!Object.hasOwn(names, glyph)) {
		throw new Error(
			`${label} has no canonical name for "${glyph}" in ${NAMES_PACKAGE}`,
		);
	}
	const entry = names[glyph];
	requireObject(entry, `${label} canonical metadata`);
	if (typeof entry.name !== "string" || entry.name === "") {
		throw new Error(`${label} has an empty or non-string canonical name`);
	}

	if (!Object.hasOwn(keywords, glyph)) {
		throw new Error(
			`${label} has no keywords for "${glyph}" in ${KEYWORDS_PACKAGE}`,
		);
	}
	const words = keywords[glyph];
	requireStringArray(words, `${label} keywords`);
	if (words.length === 0) {
		throw new Error(`${label} has an empty keyword list`);
	}

	return { canonicalName: entry.name, keywords: [...new Set(words)] };
}

function validateRecord(record, index) {
	if (record === null || typeof record !== "object" || Array.isArray(record)) {
		throw new Error(`record ${index} is not an object`);
	}

	const label = recordLabel(record, index);
	requireFields(record, RECORD_FIELDS, label);
	requireStringFields(record, RECORD_STRING_FIELDS, label);
	if (
		typeof record.sort_order !== "number" ||
		!Number.isFinite(record.sort_order)
	) {
		throw new Error(`${label} has a non-numeric "sort_order"`);
	}
	if (typeof record.has_img_apple !== "boolean") {
		throw new Error(`${label} has a non-boolean "has_img_apple"`);
	}
	if (record.short_names != null) {
		requireStringArray(record.short_names, `${label} "short_names"`);
	}
	if (record.text != null && typeof record.text !== "string") {
		throw new Error(`${label} has a non-string "text"`);
	}
	if (record.texts != null) {
		requireStringArray(record.texts, `${label} "texts"`);
	}

	if (record.skin_variations != null) {
		requireObject(record.skin_variations, `${label} "skin_variations"`);
		for (const [tone, skin] of Object.entries(record.skin_variations)) {
			const skinLabel = `${label} skin "${tone}"`;
			requireFields(skin, SKIN_FIELDS, skinLabel);
			requireStringFields(skin, SKIN_STRING_FIELDS, skinLabel);
			if (typeof skin.has_img_apple !== "boolean") {
				throw new Error(`${skinLabel} has a non-boolean "has_img_apple"`);
			}
		}
	}

	return record;
}

/**
 * Convert `emoji-datasource-apple` records into the runtime manifest: Apple
 * images only, skin tone components dropped, ordered by upstream sort order,
 * the legacy Unicode name replaced by the canonical English one, search
 * keywords attached, and empty fields omitted to keep the payload small.
 */
export function normalizeEmojiData(records, version) {
	if (!Array.isArray(records)) {
		throw new Error("emoji datasource is not an array");
	}

	const emojis = records
		.map(validateRecord)
		.filter(
			(record) =>
				record.has_img_apple && record.category !== EXCLUDED_CATEGORY,
		)
		.sort((a, b) => a.sort_order - b.sort_order)
		.map((record, index) => {
			const label = recordLabel(record, index);
			const unified = record.unified.toLowerCase();
			// Join key into the metadata packages, not a manifest field.
			const glyph = emojiGlyph(unified, label);
			const { canonicalName, keywords } = emojiMetadata(glyph, label);
			const aliases = (record.short_names ?? []).filter(
				(alias) => alias !== record.short_name,
			);
			const textAliases = [
				...new Set([record.text, ...(record.texts ?? [])].filter(Boolean)),
			];
			const skins = Object.keys(record.skin_variations ?? {})
				.sort()
				.filter((tone) => record.skin_variations[tone].has_img_apple)
				.map((tone) => ({
					tone: tone.toLowerCase(),
					unified: record.skin_variations[tone].unified.toLowerCase(),
					image: record.skin_variations[tone].image,
				}));

			return {
				id: record.short_name,
				name: canonicalName,
				category: record.category,
				subcategory: record.subcategory,
				unified,
				image: record.image,
				keywords,
				...(aliases.length > 0 && { aliases }),
				...(textAliases.length > 0 && { textAliases }),
				...(skins.length > 0 && { skins }),
			};
		});

	const categories = [...new Set(emojis.map((emoji) => emoji.category))];
	return { version, categories, emojis };
}

/**
 * Serialize the manifest through the installed Biome so the artifact stays
 * covered by `pnpm check`. Biome preserves the caller's object expansion, so
 * the indented input keeps the output stable between runs.
 */
function renderManifest(manifest) {
	return execFileSync(
		process.execPath,
		[
			require.resolve("@biomejs/biome/bin/biome"),
			"format",
			`--stdin-file-path=${MANIFEST_RELATIVE}`,
		],
		{
			cwd: siteDir,
			input: JSON.stringify(manifest, null, "\t"),
			maxBuffer: 64 * 1024 * 1024,
			encoding: "utf8",
		},
	);
}

/** Build the manifest from the installed package. */
function buildManifest() {
	const { version } = require("emoji-datasource-apple/package.json");
	return normalizeEmojiData(require("emoji-datasource-apple"), version);
}

function errorMessage(error) {
	return error instanceof Error ? error.message : String(error);
}

function hashDirectory(dir) {
	const hashes = new Map();
	for (const name of readdirSync(dir).sort()) {
		if (name.endsWith(".png") && name !== LEGACY_SPRITESHEET) {
			hashes.set(
				name,
				createHash("sha256")
					.update(readFileSync(join(dir, name)))
					.digest("hex"),
			);
		}
	}
	return hashes;
}

/** Sync images from the installed package and regenerate the manifest. */
export function update() {
	const source = hashDirectory(SOURCE_IMAGES_DIR);
	const committed = hashDirectory(IMAGES_DIR);

	for (const name of source.keys()) {
		if (committed.get(name) !== source.get(name)) {
			copyFileSync(join(SOURCE_IMAGES_DIR, name), join(IMAGES_DIR, name));
		}
	}

	const orphans = [...committed.keys()].filter((name) => !source.has(name));
	if (orphans.length > 0) {
		console.warn(
			`${orphans.length} committed image(s) are absent from the datasource. ` +
				"They are not removed automatically because /emojis/*.png are public URLs. " +
				"Review each one and delete it by hand once you know nothing references it:",
		);
		for (const name of orphans) {
			console.warn(`  - static/emojis/${name}`);
		}
	}

	const manifest = buildManifest();
	writeFileSync(MANIFEST_PATH, renderManifest(manifest));
	console.log(
		`Synced ${source.size} images and wrote ${manifest.emojis.length} emoji to ` +
			`${MANIFEST_RELATIVE} (emoji-datasource-apple@${manifest.version}).`,
	);
}

/** Verify the committed artifacts against the installed package. */
export function check() {
	const problems = [];

	const source = hashDirectory(SOURCE_IMAGES_DIR);
	const committed = hashDirectory(IMAGES_DIR);
	const report = (names, message) => {
		if (names.length > 0) {
			const sample = names.slice(0, 5).join(", ");
			problems.push(
				`${names.length} image(s) ${message}: ${sample}${names.length > 5 ? ", ..." : ""}`,
			);
		}
	};
	report(
		[...source.keys()].filter((name) => !committed.has(name)),
		"missing from static/emojis, run `pnpm emojis`",
	);
	report(
		[...source.keys()].filter(
			(name) => committed.has(name) && committed.get(name) !== source.get(name),
		),
		"differ from the datasource, run `pnpm emojis`",
	);
	report(
		[...committed.keys()].filter((name) => !source.has(name)),
		"absent from the datasource; `pnpm emojis` will not delete them, so review and remove them by hand",
	);

	if (readFileSync(MANIFEST_PATH, "utf8") !== renderManifest(buildManifest())) {
		problems.push(`${MANIFEST_RELATIVE} is stale, run \`pnpm emojis\``);
	}

	if (problems.length > 0) {
		console.error("Emoji artifacts are out of date:");
		for (const problem of problems) {
			console.error(`  - ${problem}`);
		}
		return false;
	}

	console.log(
		`Emoji artifacts match emoji-datasource-apple: ${source.size} images.`,
	);
	return true;
}

if (process.argv[1] === scriptPath) {
	const args = process.argv.slice(2);
	const unknown = args.filter((arg) => arg !== "--check");
	if (unknown.length > 0) {
		console.error(`Unknown argument: ${unknown[0]}`);
		console.error("Usage: node scripts/update-emojis.mjs [--check]");
		process.exitCode = 1;
	} else {
		try {
			if (args.includes("--check")) {
				process.exitCode = check() ? 0 : 1;
			} else {
				update();
			}
		} catch (error) {
			console.error(errorMessage(error));
			process.exitCode = 1;
		}
	}
}
