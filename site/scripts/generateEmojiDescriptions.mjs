import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const data = require("@emoji-mart/data/sets/15/apple.json");
const skinTones = {
	"1f3fb": "light skin tone",
	"1f3fc": "medium-light skin tone",
	"1f3fd": "medium skin tone",
	"1f3fe": "medium-dark skin tone",
	"1f3ff": "dark skin tone",
};

const descriptions = {};
for (const emoji of Object.values(data.emojis)) {
	for (const skin of emoji.skins) {
		// Some shipped filenames omit the emoji presentation selector.
		const unified = skin.unified.replace(/-fe0f/g, "");
		const tones = skin.unified.split("-").flatMap((code) => skinTones[code] ?? []);
		const name = tones.length ? `${emoji.name}: ${tones.join(", ")}` : emoji.name;
		descriptions[unified] = `${name} emoji`;
	}
}

process.stdout.write(`${JSON.stringify(descriptions, null, "\t")}\n`);
