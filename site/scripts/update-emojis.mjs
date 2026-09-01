/**
 * Emoji asset pipeline.
 *
 * `static/emojis/*.png` are public URLs (`/emojis/1f4bb.png`) referenced by
 * templates, proxies and agent log sources, so the file set is an API surface.
 * They are synchronized from the pinned `emoji-datasource-apple` devDependency.
 * The picker manifest is derived from the same package at bundle time instead
 * of being stored in the repository.
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
 *   node scripts/update-emojis.mjs           # sync images
 *   node scripts/update-emojis.mjs --check   # verify committed images and metadata
 */
import { createHash } from "node:crypto";
import { copyFileSync, readFileSync, readdirSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

// Vitest rewrites `new URL("..", import.meta.url)` to its dev server origin,
// so derive the site directory from the file path instead.
const scriptPath = fileURLToPath(import.meta.url);
const siteDir = dirname(dirname(scriptPath));

const require = createRequire(scriptPath);
export const IMAGES_DIR = join(siteDir, "static/emojis");
const LEGACY_SPRITESHEET = "spritesheet.png";

// Skin tone modifiers are not selectable emoji.
const EXCLUDED_CATEGORY = "Component";

// Metadata packages, both keyed by the rendered emoji glyph.
const NAMES_PACKAGE = "unicode-emoji-json";
const KEYWORDS_PACKAGE = "emojilib";
let canonicalNames;
let searchKeywords;

function getCanonicalNames() {
	canonicalNames ??= require("unicode-emoji-json/data-by-emoji.json");
	return canonicalNames;
}

function getSearchKeywords() {
	searchKeywords ??= require("emojilib");
	return searchKeywords;
}

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
	names = getCanonicalNames(),
	keywords = getSearchKeywords(),
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

/** Build the runtime manifest from the installed packages. */
export function buildManifest() {
	const { version } = require("emoji-datasource-apple/package.json");
	return normalizeEmojiData(require("emoji-datasource-apple"), version);
}

function errorMessage(error) {
	return error instanceof Error ? error.message : String(error);
}

function sourceImagesDir() {
	const packageDir = dirname(require.resolve("emoji-datasource-apple"));
	return join(packageDir, "img/apple/64");
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

/** Sync images from the installed package and validate the runtime metadata. */
export function update() {
	const sourceImages = sourceImagesDir();
	const source = hashDirectory(sourceImages);
	const committed = hashDirectory(IMAGES_DIR);

	for (const name of source.keys()) {
		if (committed.get(name) !== source.get(name)) {
			copyFileSync(join(sourceImages, name), join(IMAGES_DIR, name));
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
	console.log(
		`Synced ${source.size} images and validated ${manifest.emojis.length} emoji ` +
			`(emoji-datasource-apple@${manifest.version}).`,
	);
}

/** Verify committed images and runtime metadata against installed packages. */
export function check() {
	const problems = [];

	const source = hashDirectory(sourceImagesDir());
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

	let manifest;
	try {
		manifest = buildManifest();
	} catch (error) {
		problems.push(`runtime metadata is invalid: ${errorMessage(error)}`);
	}

	if (problems.length > 0) {
		console.error("Emoji artifacts are out of date:");
		for (const problem of problems) {
			console.error(`  - ${problem}`);
		}
		return false;
	}

	if (manifest === undefined) {
		return false;
	}

	console.log(
		`Emoji artifacts match emoji-datasource-apple: ${source.size} images and ` +
			`${manifest.emojis.length} emoji.`,
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
