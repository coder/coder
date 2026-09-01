export type SheetPosition = {
	sheetX: number;
	sheetY: number;
};

export type SkinVariation = SheetPosition & {
	tone: string;
	unified: string;
	image: string;
};

export type Emoji = SheetPosition & {
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

export type EmojiSheet = {
	file: string;
	hash: string;
	columns: number;
	rows: number;
};

export type EmojiManifest = {
	version: string;
	sheet: EmojiSheet;
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
	sheet: { columns: number; rows: number; hash: string },
): EmojiManifest;

export function readSheetGeometry(path: string): {
	columns: number;
	rows: number;
};

export function buildManifest(): EmojiManifest;

export function update(): void;

export function check(): boolean;
