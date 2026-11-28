import { readdirSync } from "node:fs";
import { createRequire } from "node:module";
import { normalizeEmojiCode } from "../src/theme/normalizeEmojiCode.mjs";

const require = createRequire(import.meta.url);
const data = require("emojibase-data/en/data.json");

const labels = new Map(
	data.flatMap((emoji) =>
		[emoji, ...(emoji.skins ?? [])].map((variant) => [
			normalizeEmojiCode(variant.hexcode),
			variant.label,
		]),
	),
);

const descriptions = {};
for (const file of readdirSync(
	new URL("../static/emojis/", import.meta.url),
).sort()) {
	if (!/^[0-9a-f-]+\.png$/.test(file)) {
		continue;
	}
	const code = normalizeEmojiCode(file.slice(0, -4));
	const label = labels.get(code);
	if (!label) {
		throw new Error(`No Emojibase description for shipped emoji ${file}`);
	}
	descriptions[code] = `${label} emoji`;
}

process.stdout.write(`${JSON.stringify(descriptions, null, "\t")}\n`);
