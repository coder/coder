import { readdirSync } from "node:fs";
import canonicalNames from "unicode-emoji-json/data-by-emoji.json";
import { describe, expect, it } from "vitest";
import manifest from "#/components/IconField/emojiDataGenerated.json";
import {
	IMAGES_DIR,
	check,
	emojiGlyph,
	emojiMetadata,
	normalizeEmojiData,
} from "./update-emojis.mjs";

const record = (overrides) => ({
	name: "GRINNING FACE",
	unified: "1F600",
	image: "1f600.png",
	short_name: "grinning",
	short_names: ["grinning"],
	text: null,
	texts: null,
	category: "Smileys & Emotion",
	subcategory: "face-smiling",
	sort_order: 1,
	has_img_apple: true,
	...overrides,
});

const skin = (unified, overrides) => ({
	unified,
	image: `${unified.toLowerCase()}.png`,
	has_img_apple: true,
	...overrides,
});

const normalize = (records) => normalizeEmojiData(records, "15.1.2").emojis;

// 😀 U+1F600, the glyph every `record()` resolves to.
const GRINNING = "\u{1f600}";

describe("normalizeEmojiData", () => {
	it("emits the runtime shape with lowercased codepoints", () => {
		const { version, categories, emojis } = normalizeEmojiData(
			[record()],
			"15.1.2",
		);
		expect(version).toBe("15.1.2");
		expect(categories).toEqual(["Smileys & Emotion"]);
		expect(emojis).toEqual([
			{
				id: "grinning",
				name: "grinning face",
				category: "Smileys & Emotion",
				subcategory: "face-smiling",
				unified: "1f600",
				image: "1f600.png",
				keywords: [
					"grinning_face",
					"face",
					"smile",
					"happy",
					"joy",
					":D",
					"grin",
					"smiley",
				],
			},
		]);
	});

	it("orders emoji and categories by sort_order", () => {
		const { categories, emojis } = normalizeEmojiData(
			[
				record({ short_name: "b", sort_order: 3, category: "Flags" }),
				record({ short_name: "a", sort_order: 2 }),
				record({ short_name: "c", sort_order: 1, category: "Objects" }),
			],
			"15.1.2",
		);
		expect(emojis.map((emoji) => emoji.id)).toEqual(["c", "a", "b"]);
		expect(categories).toEqual(["Objects", "Smileys & Emotion", "Flags"]);
	});

	it("drops the Component category and non-Apple emoji", () => {
		expect(
			normalize([
				record({ short_name: "skin-tone-2", category: "Component" }),
				record({ short_name: "male_sign", has_img_apple: false }),
				record({ short_name: "grinning" }),
			]).map((emoji) => emoji.id),
		).toEqual(["grinning"]);
	});

	it("collects aliases and deduplicated text aliases", () => {
		const [emoji] = normalize([
			record({
				short_names: ["grinning", "satisfied"],
				text: ":D",
				texts: [":D", ":-D"],
			}),
		]);
		expect(emoji.aliases).toEqual(["satisfied"]);
		expect(emoji.textAliases).toEqual([":D", ":-D"]);
	});

	it("keeps Apple skin variants in tone order", () => {
		const [emoji] = normalize([
			record({
				skin_variations: {
					"1F3FC": skin("1F600-1F3FC"),
					"1F3FB": skin("1F600-1F3FB"),
					"1F3FF": skin("1F600-1F3FF", { has_img_apple: false }),
				},
			}),
		]);
		expect(emoji.skins).toEqual([
			{ tone: "1f3fb", unified: "1f600-1f3fb", image: "1f600-1f3fb.png" },
			{ tone: "1f3fc", unified: "1f600-1f3fc", image: "1f600-1f3fc.png" },
		]);
	});

	it("preserves composite tone keys for multi-person emoji", () => {
		const [emoji] = normalize([
			record({
				short_name: "man-heart-man",
				skin_variations: {
					"1F3FC-1F3FB": skin("1F468-1F3FC-200D-2764-FE0F-200D-1F468-1F3FB"),
					"1F3FB-1F3FB": skin("1F468-1F3FB-200D-2764-FE0F-200D-1F468-1F3FB"),
				},
			}),
		]);
		expect(emoji.skins.map((variant) => variant.tone)).toEqual([
			"1f3fb-1f3fb",
			"1f3fc-1f3fb",
		]);
		expect(emoji.skins[0].unified).toBe(
			"1f468-1f3fb-200d-2764-fe0f-200d-1f468-1f3fb",
		);
	});

	it("omits empty optional fields", () => {
		const [emoji] = normalize([record()]);
		expect(emoji).not.toHaveProperty("aliases");
		expect(emoji).not.toHaveProperty("textAliases");
		expect(emoji).not.toHaveProperty("skins");
	});

	it("replaces the legacy datasource name with the canonical one", () => {
		const [emoji] = normalize([
			record({
				short_name: "heart",
				name: "HEAVY BLACK HEART",
				unified: "2764-FE0F",
				image: "2764-fe0f.png",
			}),
		]);
		expect(emoji.name).toBe("red heart");
		expect(emoji.keywords).toContain("love");
	});

	it("does not emit the glyph join key", () => {
		const [emoji] = normalize([record()]);
		expect(emoji).not.toHaveProperty("glyph");
		expect(emoji).not.toHaveProperty("canonicalName");
	});
});

describe("emojiGlyph", () => {
	it("renders a single codepoint", () => {
		expect(emojiGlyph("1f600", "test")).toBe(GRINNING);
	});

	it("renders a joined sequence in order", () => {
		expect(emojiGlyph("1f3f3-fe0f-200d-26a7-fe0f", "test")).toBe(
			"\u{1f3f3}\ufe0f\u200d\u26a7\ufe0f",
		);
	});

	it("rejects a non-hexadecimal codepoint", () => {
		expect(() => emojiGlyph("1f600-zzz", "test")).toThrow(
			'test has a non-hexadecimal codepoint "zzz"',
		);
	});

	it("rejects a codepoint above the Unicode range", () => {
		expect(() => emojiGlyph("110000", "test")).toThrow(
			'test has an out-of-range codepoint "110000"',
		);
	});
});

describe("emojiMetadata", () => {
	const names = { [GRINNING]: { name: "grinning face" } };
	const keywords = { [GRINNING]: ["face", "smile", "face"] };

	it("returns the canonical name and deduplicated keywords", () => {
		expect(emojiMetadata(GRINNING, "test", names, keywords)).toEqual({
			canonicalName: "grinning face",
			keywords: ["face", "smile"],
		});
	});

	it("reads the real packages by default", () => {
		const { canonicalName, keywords: words } = emojiMetadata(GRINNING, "test");
		expect(canonicalName).toBe("grinning face");
		expect(words).toContain("happy");
	});

	it("rejects a glyph the name package does not know", () => {
		expect(() => emojiMetadata(GRINNING, "test", {}, keywords)).toThrow(
			"test has no canonical name",
		);
	});

	it("rejects a malformed name entry", () => {
		expect(() =>
			emojiMetadata(GRINNING, "test", { [GRINNING]: "grinning" }, keywords),
		).toThrow("test canonical metadata is not an object");
	});

	it("rejects an empty or non-string canonical name", () => {
		expect(() =>
			emojiMetadata(GRINNING, "test", { [GRINNING]: { name: "" } }, keywords),
		).toThrow("test has an empty or non-string canonical name");
		expect(() =>
			emojiMetadata(GRINNING, "test", { [GRINNING]: { name: 7 } }, keywords),
		).toThrow("test has an empty or non-string canonical name");
	});

	it("rejects a glyph the keyword package does not know", () => {
		expect(() => emojiMetadata(GRINNING, "test", names, {})).toThrow(
			"test has no keywords",
		);
	});

	it("rejects keywords that are not an array of strings", () => {
		expect(() =>
			emojiMetadata(GRINNING, "test", names, { [GRINNING]: ["face", 7] }),
		).toThrow("test keywords is not an array of strings");
	});

	it("rejects an empty keyword list", () => {
		expect(() =>
			emojiMetadata(GRINNING, "test", names, { [GRINNING]: [] }),
		).toThrow("test has an empty keyword list");
	});
});

describe("normalizeEmojiData validation", () => {
	it("rejects a datasource that is not an array", () => {
		expect(() => normalizeEmojiData({}, "15.1.2")).toThrow(
			"emoji datasource is not an array",
		);
	});

	it("rejects a non-object record", () => {
		expect(() => normalize([null])).toThrow("record 0 is not an object");
	});

	it("rejects a record missing a required field", () => {
		expect(() => normalize([record({ subcategory: undefined })])).toThrow(
			'emoji "grinning" is missing "subcategory"',
		);
	});

	it("validates records before filtering them", () => {
		expect(() =>
			normalize([record({ has_img_apple: false, unified: undefined })]),
		).toThrow('emoji "grinning" is missing "unified"');
	});

	it("rejects a non-string required field", () => {
		expect(() => normalize([record({ image: 123 })])).toThrow(
			'emoji "grinning" has a non-string "image"',
		);
	});

	it("rejects a non-numeric sort_order", () => {
		expect(() => normalize([record({ sort_order: "1" })])).toThrow(
			'emoji "grinning" has a non-numeric "sort_order"',
		);
	});

	it("rejects a skin variation container that is not an object", () => {
		expect(() => normalize([record({ skin_variations: [] })])).toThrow(
			'emoji "grinning" "skin_variations" is not an object',
		);
	});

	it("rejects a skin variation missing a required field", () => {
		expect(() =>
			normalize([
				record({
					skin_variations: {
						"1F3FB": skin("1F600-1F3FB", { image: null }),
					},
				}),
			]),
		).toThrow('emoji "grinning" skin "1F3FB" is missing "image"');
	});

	it("rejects a datasource emoji with no canonical metadata", () => {
		expect(() => normalize([record({ unified: "0041" })])).toThrow(
			'emoji "grinning" has no canonical name for "A"',
		);
	});

	it("rejects a datasource emoji with an unparsable codepoint", () => {
		expect(() => normalize([record({ unified: "NOPE" })])).toThrow(
			'emoji "grinning" has a non-hexadecimal codepoint "nope"',
		);
	});
});

describe("committed emoji artifacts", () => {
	it("match the installed datasource", () => {
		expect(check()).toBe(true);
	});

	it("reference images that exist on disk", () => {
		const committed = new Set(readdirSync(IMAGES_DIR));
		const referenced = new Set(
			manifest.emojis.flatMap((emoji) => [
				emoji.image,
				...(emoji.skins ?? []).map((variant) => variant.image),
			]),
		);
		expect(referenced.size).toBeGreaterThan(0);
		expect([...referenced].filter((image) => !committed.has(image))).toEqual([]);
	});

	it("carry the canonical name and keywords for every emoji", () => {
		const stale = manifest.emojis.filter((emoji) => {
			const entry = canonicalNames[emojiGlyph(emoji.unified, emoji.id)];
			return (
				entry?.name !== emoji.name ||
				!Array.isArray(emoji.keywords) ||
				emoji.keywords.length === 0
			);
		});
		expect(stale.map((emoji) => emoji.id)).toEqual([]);
	});

	it("omit the glyph join key", () => {
		const withGlyph = manifest.emojis.filter(
			(emoji) => "glyph" in emoji || "canonicalName" in emoji,
		);
		expect(withGlyph.map((emoji) => emoji.id)).toEqual([]);
	});

	it("resolve searches that the legacy datasource names cannot", () => {
		const find = (query) =>
			manifest.emojis.filter((emoji) =>
				[emoji.name, ...emoji.keywords].join(" ").toLowerCase().includes(query),
			);
		expect(find("red heart").map((emoji) => emoji.id)).toContain("heart");
		expect(find("happy").length).toBeGreaterThan(10);
		expect(find("sad").length).toBeGreaterThan(10);
	});
});
