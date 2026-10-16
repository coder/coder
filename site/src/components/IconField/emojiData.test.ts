import { readdirSync } from "node:fs";
import path from "node:path";
import { DEPRECATED_ICONS } from "#/theme/deprecatedIcons";
import { buildEmojiData, emojiToValue, ICONS_CATEGORY } from "./emojiData";

const emojisDir = path.resolve(import.meta.dirname, "../../../static/emojis");
const shippedFiles = readdirSync(emojisDir).filter((file) =>
	file.endsWith(".png"),
);

describe("emojiData", () => {
	it("maps every emoji and skin tone to a shipped PNG", () => {
		const shipped = new Set(shippedFiles);
		const missing: string[] = [];
		for (const entry of buildEmojiData().emojis) {
			if (entry.category === ICONS_CATEGORY) {
				continue;
			}
			for (const emoji of [entry.emoji, ...Object.values(entry.skins ?? {})]) {
				const value = emojiToValue(emoji);
				if (!shipped.has(value.replace("/emojis/", ""))) {
					missing.push(`${entry.label}: ${emoji}`);
				}
			}
		}
		expect(missing).toEqual([]);
	});

	it("keeps FE0F only after text-presentation characters", () => {
		expect(emojiToValue("\u{1F93C}\u200D\u2642\uFE0F")).toBe(
			"/emojis/1f93c-200d-2642-fe0f.png",
		);
		expect(emojiToValue("\u263A\uFE0F")).toBe("/emojis/263a-fe0f.png");
		expect(emojiToValue("\u{1F610}\uFE0F")).toBe("/emojis/1f610.png");
		expect(emojiToValue("/icon/docker.svg")).toBe("/icon/docker.svg");
	});

	it("lists non-deprecated icons in the last category", () => {
		const data = buildEmojiData();
		const icons = data.emojis.filter(
			(entry) => entry.category === ICONS_CATEGORY,
		);

		expect(data.categories.at(-1)).toEqual({
			index: ICONS_CATEGORY,
			label: "Icons",
		});
		expect(icons.map((entry) => entry.emoji)).toContain("/icon/docker.svg");
		for (const icon of DEPRECATED_ICONS) {
			expect(icons.map((entry) => entry.emoji)).not.toContain(`/icon/${icon}`);
		}
	});
});
