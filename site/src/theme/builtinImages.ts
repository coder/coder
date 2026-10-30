import icons from "./builtinIcons.json";
import emojiDescriptions from "./emojiDescriptionsGenerated.json";

const iconDescriptions = new Map(
	icons.flatMap((icon) =>
		icon.sources.map((src) => [src, icon.description] as const),
	),
);
const emojiDescriptionsByCode = new Map(Object.entries(emojiDescriptions));

/**
 * Describes a built-in, root-relative asset URL, or returns undefined if unknown.
 * Callers decide whether the image is informative or decorative in their context.
 */
export function getBuiltinImageDescription(
	src: string | undefined,
): string | undefined {
	if (!src) {
		return undefined;
	}
	const path = src.split(/[?#]/)[0];
	const icon = iconDescriptions.get(path);
	if (icon) {
		return icon;
	}
	const emoji = /^\/emojis\/([0-9a-f-]+)\.png$/.exec(path);
	return emoji
		? emojiDescriptionsByCode.get(emoji[1].replace(/-fe0f/g, ""))
		: undefined;
}
