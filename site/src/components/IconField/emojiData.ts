import emojibaseEmojis from "emojibase-data/en/compact.json";
import emojibaseMessages from "emojibase-data/en/messages.json";
import type { EmojiData } from "frimousse";
import { DEPRECATED_ICONS } from "#/theme/deprecatedIcons";
import icons from "#/theme/icons.json";
import emojiFiles from "./emojiFiles.json";

const COMPONENT_GROUP = 2;
/** Category index for icons.json, placed after every Emojibase group. */
export const ICONS_CATEGORY = 100;

type SkinToneKey = keyof EmojiData["skinTones"];

const SKIN_TONE_MODIFIERS: Record<SkinToneKey, string> = {
	light: "\u{1F3FB}",
	"medium-light": "\u{1F3FC}",
	medium: "\u{1F3FD}",
	"medium-dark": "\u{1F3FE}",
	dark: "\u{1F3FF}",
};
const ALL_MODIFIERS = /[\u{1F3FB}-\u{1F3FF}]/gu;

const availableFiles = new Set<string>(emojiFiles);

const toHex = (emoji: string) =>
	Array.from(emoji, (char) =>
		(char.codePointAt(0) ?? 0).toString(16).padStart(4, "0"),
	).join("-");

/**
 * Returns the name of the shipped PNG for an emoji, without extension.
 * emoji-datasource keeps FE0F in some file names ("1f93c-200d-2642-fe0f")
 * and drops it in others ("1f610"), so both forms are checked against the
 * list of shipped files.
 */
const emojiFile = (emoji: string): string | undefined => {
	for (const name of [toHex(emoji), toHex(emoji.replaceAll("\uFE0F", ""))]) {
		if (availableFiles.has(name)) {
			return name;
		}
	}
	return undefined;
};

/**
 * Returns the value IconField stores for a picked entry: custom icons are
 * already paths, and Unicode emoji map to their Apple PNG.
 */
export const emojiToValue = (emoji: string): string => {
	if (emoji.startsWith("/")) {
		return emoji;
	}
	const file = emojiFile(emoji);
	return file ? `/emojis/${file}.png` : "";
};

const singleToneSkin = (
	skins: readonly { unicode: string }[] | undefined,
	tone: SkinToneKey,
): string | undefined => {
	const modifier = SKIN_TONE_MODIFIERS[tone];
	const unicode = skins?.find((skin) => {
		const modifiers = skin.unicode.match(ALL_MODIFIERS) ?? [];
		return modifiers.length > 0 && modifiers.every((m) => m === modifier);
	})?.unicode;
	return unicode && emojiFile(unicode) ? unicode : undefined;
};

/** Returns all five single-tone variants, or undefined if any lacks a PNG. */
const skinVariants = (
	skins: readonly { unicode: string }[] | undefined,
): EmojiData["skinTones"] | undefined => {
	const light = singleToneSkin(skins, "light");
	const mediumLight = singleToneSkin(skins, "medium-light");
	const medium = singleToneSkin(skins, "medium");
	const mediumDark = singleToneSkin(skins, "medium-dark");
	const dark = singleToneSkin(skins, "dark");
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

const skinToneLabel = (tone: SkinToneKey) =>
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
			!emojiFile(entry.unicode)
		) {
			continue;
		}
		emojis.push({
			emoji: entry.unicode,
			category: entry.group,
			label: entry.label,
			// Frimousse only uses version to filter its own CDN data.
			version: 0,
			tags: entry.tags ?? [],
			skins: skinVariants(entry.skins),
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
