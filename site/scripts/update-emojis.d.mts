export type SkinVariation = {
	tone: string;
	unified: string;
	image: string;
};

export type Emoji = {
	id: string;
	name: string;
	category: string;
	subcategory: string;
	unified: string;
	image: string;
	keywords: string[];
	aliases?: string[];
	textAliases?: string[];
	skins?: SkinVariation[];
};

export type EmojiManifest = {
	version: string;
	categories: string[];
	emojis: Emoji[];
};

export const IMAGES_DIR: string;

export function emojiGlyph(unified: string, label: string): string;

export function emojiMetadata(
	glyph: string,
	label: string,
	names?: Record<string, { name: string }>,
	keywords?: Record<string, string[]>,
): { canonicalName: string; keywords: string[] };

export function normalizeEmojiData(
	records: unknown,
	version: string,
): EmojiManifest;

export function buildManifest(): EmojiManifest;

export function update(): void;

export function check(): boolean;
