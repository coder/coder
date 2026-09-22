import type { Chat } from "#/api/typesGenerated";

// Every label this feature writes starts with this prefix so it can be
// told apart from labels other tools put on a chat.
const BOARD_LABEL_PREFIX = "board/";

const COLUMN_KEY = `${BOARD_LABEL_PREFIX}column`;
const GROUP_KEY = `${BOARD_LABEL_PREFIX}group`;
const TITLE_KEY = `${BOARD_LABEL_PREFIX}title`;
const COLOR_KEY = `${BOARD_LABEL_PREFIX}color`;
const POSITION_KEY = `${BOARD_LABEL_PREFIX}pos`;
const COMMENT_PREFIX = `${BOARD_LABEL_PREFIX}comment.`;
/** On a card's assistant chat: the card id. Such chats are not cards themselves. */
export const ASSISTANT_KEY = `${BOARD_LABEL_PREFIX}assistant`;

// Server limit on a label value, see coderd/httpapi/chatlabels.go.
const MAX_LABEL_VALUE_BYTES = 256;

export const INBOX_COLUMN = "Inbox";

/** A column's hue; the components map it to theme classes. Inbox is always neutral. */
type ColumnHue =
	| "neutral"
	| "purple"
	| "sky"
	| "green"
	| "orange"
	| "magenta"
	| "red";

const COLUMN_HUES: readonly ColumnHue[] = [
	"purple",
	"sky",
	"green",
	"orange",
	"magenta",
	"red",
];

// Hashing the name keeps the board and the sidebar in agreement without
// storing a color anywhere.
export const columnHue = (name: string): ColumnHue => {
	if (name === INBOX_COLUMN) return "neutral";
	let hash = 0;
	for (const char of name)
		hash = (hash * 31 + (char.codePointAt(0) ?? 0)) >>> 0;
	return COLUMN_HUES[hash % COLUMN_HUES.length] ?? "neutral";
};

// Card tints map onto the theme's tinted surfaces (bg-surface-<name>), which
// already carry dark and light variants. Decorative only, never status.
export const CARD_COLORS = [
	"green",
	"orange",
	"sky",
	"red",
	"purple",
	"magenta",
] as const;

export type CardColor = (typeof CARD_COLORS)[number];

const isCardColor = (value: string | undefined): value is CardColor =>
	value !== undefined && (CARD_COLORS as readonly string[]).includes(value);

export type BoardNote = Readonly<{
	index: number;
	timestamp: number;
	text: string;
}>;

type BoardComment = BoardNote;

export type BoardCard = Readonly<{
	/** The primary chat id. Comments and title live on this chat. */
	id: string;
	title: string;
	column: string;
	color: CardColor | undefined;
	primary: Chat;
	/** Primary first, then the rest by creation. */
	members: readonly Chat[];
	comments: readonly BoardComment[];
}>;

export type BoardColumn = Readonly<{
	name: string;
	cards: readonly BoardCard[];
}>;

export const getColumnLabel = (chat: Chat): string =>
	chat.labels[COLUMN_KEY] ?? INBOX_COLUMN;

/** The user-given card title on a primary chat, if any. */
export const getTitleLabel = (chat: Chat): string | undefined =>
	chat.labels[TITLE_KEY];

/** The card color set on a primary chat, if any. */
const getColorLabel = (chat: Chat): CardColor | undefined => {
	const value = chat.labels[COLOR_KEY];
	return isCardColor(value) ? value : undefined;
};

export const getGroupLabel = (chat: Chat): string =>
	chat.labels[GROUP_KEY] ?? chat.id;

const commentKeyPattern = /^board\/comment\.(\d+)\.(timestamp|\d+)$/;

export const parseComments = (
	labels: Record<string, string>,
): BoardComment[] => {
	const byIndex = new Map<number, { timestamp: number; chunks: string[] }>();
	for (const [key, value] of Object.entries(labels)) {
		const match = commentKeyPattern.exec(key);
		if (!match) continue;
		const index = Number(match[1]);
		const entry = byIndex.get(index) ?? { timestamp: 0, chunks: [] };
		if (match[2] === "timestamp") {
			const ts = Number(value);
			entry.timestamp = Number.isFinite(ts) ? ts : 0;
		} else {
			entry.chunks[Number(match[2])] = value;
		}
		byIndex.set(index, entry);
	}
	return [...byIndex.entries()]
		.sort(([a], [b]) => a - b)
		.map(([index, entry]) => ({
			index,
			timestamp: entry.timestamp,
			text: entry.chunks.join(""),
		}));
};

// Splits on UTF-8 byte length, never inside a multi-byte character, because
// the server measures label values in bytes.
export const chunkByBytes = (text: string): string[] => {
	const encoder = new TextEncoder();
	const chunks: string[] = [];
	let current = "";
	for (const char of text) {
		const candidate = current + char;
		if (encoder.encode(candidate).length > MAX_LABEL_VALUE_BYTES) {
			chunks.push(current);
			current = char;
		} else {
			current = candidate;
		}
	}
	if (current || chunks.length === 0) {
		chunks.push(current);
	}
	return chunks;
};

export const commentLabels = (
	index: number,
	text: string,
	timestamp: number,
): Record<string, string> => {
	const out: Record<string, string> = {
		[`${COMMENT_PREFIX}${index}.timestamp`]: String(timestamp),
	};
	for (const [i, chunk] of chunkByBytes(text).entries()) {
		out[`${COMMENT_PREFIX}${index}.${i}`] = chunk;
	}
	return out;
};

export const nextCommentIndex = (comments: readonly BoardComment[]): number =>
	comments.reduce((max, c) => Math.max(max, c.index + 1), 0);

const withoutKeys = (
	labels: Record<string, string>,
	predicate: (key: string) => boolean,
): Record<string, string> =>
	Object.fromEntries(Object.entries(labels).filter(([k]) => !predicate(k)));

export const setColumnLabel = (
	labels: Record<string, string>,
	column: string,
): Record<string, string> => {
	const rest = withoutKeys(labels, (k) => k === COLUMN_KEY);
	return column === INBOX_COLUMN ? rest : { ...rest, [COLUMN_KEY]: column };
};

export const setGroupLabel = (
	labels: Record<string, string>,
	primaryId: string,
	selfId: string,
): Record<string, string> => {
	const rest = withoutKeys(labels, (k) => k === GROUP_KEY);
	return primaryId === selfId ? rest : { ...rest, [GROUP_KEY]: primaryId };
};

export const setTitleLabel = (
	labels: Record<string, string>,
	title: string,
): Record<string, string> => {
	const rest = withoutKeys(labels, (k) => k === TITLE_KEY);
	return title.trim() ? { ...rest, [TITLE_KEY]: title.trim() } : rest;
};

export const setColorLabel = (
	labels: Record<string, string>,
	color: CardColor | undefined,
): Record<string, string> => {
	const rest = withoutKeys(labels, (k) => k === COLOR_KEY);
	return color ? { ...rest, [COLOR_KEY]: color } : rest;
};

/** Records when the user placed the card so its position stops following chat activity. */
export const setPositionLabel = (
	labels: Record<string, string>,
	placedAt = Date.now(),
): Record<string, string> => ({ ...labels, [POSITION_KEY]: String(placedAt) });

// A card sorts by its placement key: the board/pos the user last gave it,
// or chat creation time when never placed. Neither changes with activity.
// Higher sorts first.
export const placementKey = (chat: Chat): number => {
	const placed = Number(chat.labels[POSITION_KEY]);
	return Number.isFinite(placed) && placed > 0
		? placed
		: new Date(chat.created_at).getTime();
};

// Gap used when placing above the top or below the bottom card, so there is
// always room to insert between later.
const PLACEMENT_GAP_MS = 60_000;

/** A key that sorts between two neighbours; either may be absent at the column edges. */
export const keyBetween = (
	above: number | undefined,
	below: number | undefined,
): number => {
	if (above !== undefined && below !== undefined) return (above + below) / 2;
	if (above !== undefined) return above - PLACEMENT_GAP_MS;
	if (below !== undefined) return below + PLACEMENT_GAP_MS;
	return Date.now();
};

export const addCommentLabels = (
	labels: Record<string, string>,
	text: string,
	timestamp = Date.now(),
): Record<string, string> => {
	const index = nextCommentIndex(parseComments(labels));
	return { ...labels, ...commentLabels(index, text, timestamp) };
};

export const removeCommentLabels = (
	labels: Record<string, string>,
	index: number,
): Record<string, string> =>
	withoutKeys(labels, (k) => k.startsWith(`${COMMENT_PREFIX}${index}.`));

/** Replaces a comment's text in place, keeping its index and timestamp. */
export const updateCommentLabels = (
	labels: Record<string, string>,
	index: number,
	text: string,
): Record<string, string> => {
	const existing = parseComments(labels).find((c) => c.index === index);
	return {
		...removeCommentLabels(labels, index),
		...commentLabels(index, text, existing?.timestamp ?? Date.now()),
	};
};

const stripCommentLabels = (
	labels: Record<string, string>,
): Record<string, string> =>
	withoutKeys(labels, (k) => k.startsWith(COMMENT_PREFIX));

/** Replaces a card's notes with `notes` in the given order, renumbered from 0. Index is display order. */
export const setCommentsLabels = (
	labels: Record<string, string>,
	notes: readonly Pick<BoardNote, "text" | "timestamp">[],
): Record<string, string> => {
	const out = stripCommentLabels(labels);
	for (const [index, note] of notes.entries()) {
		Object.assign(out, commentLabels(index, note.text, note.timestamp));
	}
	return out;
};

/** Removes card-level data (title, comments) from a chat that stops being a primary. */
export const stripCardLabels = (
	labels: Record<string, string>,
): Record<string, string> =>
	withoutKeys(
		stripCommentLabels(labels),
		(k) =>
			k === TITLE_KEY ||
			k === GROUP_KEY ||
			k === COLOR_KEY ||
			k === POSITION_KEY,
	);

/** The card-level data a primary carries (title, color, position, comments), for handing to a new primary. */
export const takeCardLabels = (
	labels: Record<string, string>,
): Record<string, string> =>
	Object.fromEntries(
		Object.entries(labels).filter(
			([k]) =>
				k === TITLE_KEY ||
				k === COLOR_KEY ||
				k === POSITION_KEY ||
				k.startsWith(COMMENT_PREFIX),
		),
	);

const byCreation = (a: Chat, b: Chat) =>
	a.created_at.localeCompare(b.created_at);

/**
 * Assembles cards from the full chat list. A chat whose group primary is not
 * in the list is shown as its own card; the link is picked up again when the
 * primary loads.
 */
export const buildCards = (chats: readonly Chat[]): BoardCard[] => {
	const byId = new Map(chats.map((c) => [c.id, c]));
	const membersByPrimary = new Map<string, Chat[]>();
	for (const chat of chats) {
		if (chat.labels[ASSISTANT_KEY]) continue;
		const group = getGroupLabel(chat);
		const primaryId = byId.has(group) ? group : chat.id;
		const list = membersByPrimary.get(primaryId) ?? [];
		list.push(chat);
		membersByPrimary.set(primaryId, list);
	}
	const cards: BoardCard[] = [];
	for (const [primaryId, members] of membersByPrimary) {
		const primary = byId.get(primaryId);
		if (!primary) continue;
		const others = members.filter((m) => m.id !== primaryId).sort(byCreation);
		cards.push({
			id: primaryId,
			title: primary.labels[TITLE_KEY] ?? primary.title,
			column: getColumnLabel(primary),
			color: getColorLabel(primary),
			primary,
			members: [primary, ...others],
			comments: parseComments(primary.labels),
		});
	}
	// Newest placement first, so a card the user just moved lands at the top
	// of its column, and unplaced Inbox cards show newest chats first.
	return cards.sort(
		(a, b) => placementKey(b.primary) - placementKey(a.primary),
	);
};

/** Each member chat mapped to its card's color, so a window can wear it. */
export const cardColorByChat = (
	cards: readonly BoardCard[],
): Map<string, CardColor> => {
	const colors = new Map<string, CardColor>();
	for (const card of cards) {
		if (!card.color) continue;
		for (const member of card.members) colors.set(member.id, card.color);
	}
	return colors;
};

/**
 * Column order: stored order first, then columns discovered from labels in
 * first-seen order. Inbox is present by default and cannot be removed; once
 * the user has ordered columns, the stored order decides its position.
 */
export const buildColumns = (
	cards: readonly BoardCard[],
	storedOrder: readonly string[],
	emptyColumns: readonly string[],
): BoardColumn[] => {
	// The stored order is authoritative once the user has reordered; before
	// that, Inbox leads and discovered columns follow.
	const names = new Set<string>(
		storedOrder.includes(INBOX_COLUMN)
			? storedOrder
			: [INBOX_COLUMN, ...storedOrder],
	);
	for (const name of emptyColumns) names.add(name);
	for (const card of cards) names.add(card.column);
	return [...names].map((name) => ({
		name,
		cards: cards.filter((card) => card.column === name),
	}));
};
