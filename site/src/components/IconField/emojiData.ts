import type { Emoji as EmojibaseEmoji, SkinTone } from "emojibase";
import emojibaseEmojis from "emojibase-data/en/data.json";
import emojibaseMessages from "emojibase-data/en/messages.json";
import type { EmojiData } from "frimousse";
import { DEPRECATED_ICONS } from "#/theme/deprecatedIcons";
import icons from "#/theme/icons.json";

const COMPONENT_GROUP = 2;
/** Highest Emoji version in the Apple set shipped in static/emojis. */
const MAX_EMOJI_VERSION = 15.1;
/** Emoji up to MAX_EMOJI_VERSION that the Apple set ships no PNG for. */
const UNSHIPPED_EMOJI = new Set([
	"\u2640\uFE0F",
	"\u2642\uFE0F",
	"\u2695\uFE0F",
]);
/** Category index for icons.json, placed after every Emojibase group. */
export const ICONS_CATEGORY = 100;

const EMOJI_PRESENTATION = /\p{Emoji_Presentation}/u;

/**
 * Returns the static/emojis file name for an emoji, without extension.
 * emoji-datasource names files after the fully-qualified sequence, which
 * has FE0F only after characters that default to text presentation
 * ("263a-fe0f", "1f93c-200d-2642-fe0f"). Emojibase adds FE0F after every
 * character, so drop it where the preceding character is already emoji.
 */
const emojiFile = (emoji: string): string => {
	const chars = Array.from(emoji);
	return chars
		.filter(
			(char, i) =>
				!(char === "\uFE0F" && EMOJI_PRESENTATION.test(chars[i - 1] ?? "")),
		)
		.map((char) => (char.codePointAt(0) ?? 0).toString(16).padStart(4, "0"))
		.join("-");
};

/**
 * Returns the value IconField stores for a picked entry: custom icons are
 * already paths, and Unicode emoji map to their Apple PNG.
 */
export const emojiToValue = (emoji: string): string =>
	emoji.startsWith("/") ? emoji : `/emojis/${emojiFile(emoji)}.png`;

const isShipped = (emoji: { emoji: string; version: number }) =>
	emoji.version <= MAX_EMOJI_VERSION && !UNSHIPPED_EMOJI.has(emoji.emoji);

const singleToneSkin = (
	entry: EmojibaseEmoji,
	tone: SkinTone,
): string | undefined => {
	const skin = entry.skins?.find((s) => s.tone === tone);
	return skin && isShipped(skin) ? skin.emoji : undefined;
};

/** Returns all five single-tone variants, or undefined if any isn't shipped. */
const skinVariants = (
	entry: EmojibaseEmoji,
): EmojiData["skinTones"] | undefined => {
	const light = singleToneSkin(entry, 1);
	const mediumLight = singleToneSkin(entry, 2);
	const medium = singleToneSkin(entry, 3);
	const mediumDark = singleToneSkin(entry, 4);
	const dark = singleToneSkin(entry, 5);
	if (!light || !mediumLight || !medium || !mediumDark || !dark) {
		return undefined;
	}
	return {
		light,
		"medium-light": mediumLight,
		medium,
		"medium-dark": mediumDark,
		dark,
	};
};

const skinToneLabel = (tone: keyof EmojiData["skinTones"]) =>
	emojibaseMessages.skinTones.find((message) => message.key === tone)
		?.message ?? tone;

/**
 * Builds the picker data from bundled Emojibase data, so the picker never
 * fetches from a CDN. Only emoji with a shipped PNG are included. Custom
 * icons are added as entries whose `emoji` is the icon path; the picker
 * renders every entry as an image.
 */
export const buildEmojiData = (): EmojiData => {
	const emojis: EmojiData["emojis"] = [];

	for (const entry of emojibaseEmojis) {
		if (
			entry.group === undefined ||
			entry.group === COMPONENT_GROUP ||
			!isShipped(entry)
		) {
			continue;
		}
		emojis.push({
			emoji: entry.emoji,
			category: entry.group,
			label: entry.label,
			version: entry.version,
			tags: entry.tags ?? [],
			skins: skinVariants(entry),
		});
	}

	for (const icon of icons) {
		if (DEPRECATED_ICONS.includes(icon)) {
			continue;
		}
		const id = icon.split(".")[0];
		emojis.push({
			emoji: `/icon/${icon}`,
			category: ICONS_CATEGORY,
			label: id,
			version: 0,
			tags: id.split("-"),
		});
	}

	return {
		locale: "en",
		emojis,
		categories: [
			...emojibaseMessages.groups
				.filter((group) => group.order !== COMPONENT_GROUP)
				.map((group) => ({
					index: group.order,
					label: group.message.charAt(0).toUpperCase() + group.message.slice(1),
				})),
			{ index: ICONS_CATEGORY, label: "Icons" },
		],
		skinTones: {
			light: skinToneLabel("light"),
			"medium-light": skinToneLabel("medium-light"),
			medium: skinToneLabel("medium"),
			"medium-dark": skinToneLabel("medium-dark"),
			dark: skinToneLabel("dark"),
		},
	};
};
