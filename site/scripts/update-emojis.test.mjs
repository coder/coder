import { readdirSync } from "node:fs";
import { describe, expect, it } from "vitest";
import manifest from "#/components/IconField/emojiDataGenerated.json";
import {
	IMAGES_DIR,
	check,
	normalizeEmojiData,
	parsePngSize,
	sheetGrid,
	validateSpritesheet,
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
				name: "GRINNING FACE",
				category: "Smileys & Emotion",
				subcategory: "face-smiling",
				unified: "1f600",
				image: "1f600.png",
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
});

describe("parsePngSize", () => {
	it("rejects a buffer without the PNG signature", () => {
		expect(() => parsePngSize(Buffer.alloc(24))).toThrow("not a PNG file");
	});

	it("rejects a PNG whose first chunk is not IHDR", () => {
		const buffer = Buffer.alloc(24);
		Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]).copy(buffer);
		buffer.write("IDAT", 12, "ascii");
		expect(() => parsePngSize(buffer)).toThrow(
			"does not start with an IHDR chunk",
		);
	});
});

describe("sheetGrid", () => {
	it("divides a sheet into padded cells", () => {
		expect(sheetGrid({ width: 4026, height: 4026 })).toEqual({
			cols: 61,
			rows: 61,
		});
	});

	it("rejects dimensions that are not a whole number of cells", () => {
		expect(() => sheetGrid({ width: 4000, height: 4026 })).toThrow(
			"not a multiple of the 66px cell stride",
		);
	});
});

describe("committed emoji artifacts", () => {
	it("match the installed datasource", () => {
		expect(check()).toBe(true);
	});

	it("keep the 61x61 spritesheet @emoji-mart/data expects", () => {
		expect(validateSpritesheet()).toEqual({ cols: 61, rows: 61 });
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
});
