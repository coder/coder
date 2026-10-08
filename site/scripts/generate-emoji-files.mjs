// Writes the list of emoji PNGs shipped in static/emojis, which the emoji
// picker uses to map Emojibase emoji to existing files. Run from site/ after
// updating static/emojis (the update-emojis script does both).
import { readdirSync, writeFileSync } from "node:fs";

const stems = readdirSync("static/emojis")
	.filter((file) => file.endsWith(".png"))
	.map((file) => file.slice(0, -".png".length))
	.sort();

writeFileSync(
	"src/components/IconField/emojiFiles.json",
	`${JSON.stringify(stems, null, "\t")}\n`,
);
console.log(`Wrote ${stems.length} emoji file names`);
