import type { Chat } from "#/api/typesGenerated";
import {
	addCommentLabels,
	type BoardCard,
	type BoardColumn,
	type CardColor,
	commentLabels,
	getTitleLabel,
	INBOX_COLUMN,
	keyBetween,
	nextCommentIndex,
	placementKey,
	removeCommentLabels,
	setColorLabel,
	setColumnLabel,
	setCommentsLabels,
	setEffortsLabels,
	setGroupLabel,
	setPositionLabel,
	setTitleLabel,
	stripCardLabels,
	takeCardLabels,
	updateCommentLabels,
} from "./boardLabels";
import type { BoardStorage, DraftTarget } from "./boardStorage";

/**
 * What a board action means, as pure functions from the full board model to
 * a Plan. Nothing here knows about React, queries, or toasts. Commands take
 * ids and resolve them against the model, so a caller holding a filtered
 * view cannot make a command act on a subset.
 */
export type BoardState = Readonly<{
	/** Every card, unfiltered. */
	cards: readonly BoardCard[];
	/** Built from every card, in display order. */
	columns: readonly BoardColumn[];
	storage: BoardStorage;
}>;

type Write = Readonly<{ chat: Chat; labels: Record<string, string> }>;

/** The effects of one command. `undo` present means the user is offered to revert the writes. */
export type Plan = Readonly<{
	writes: readonly Write[];
	titles?: readonly Readonly<{ chat: Chat; title: string }>[];
	storage?: Partial<BoardStorage>;
	undo?: string;
}>;

/** Where a moved note lands: before or after an existing note, or at the end. */
export type NoteSlot = Readonly<{ index: number; side: "before" | "after" }>;

/** Where a moved column lands relative to another column. */
type ColumnSlot = Readonly<{ name: string; side: "before" | "after" }>;

const cardOf = (state: BoardState, cardId: string) =>
	state.cards.find((card) => card.id === cardId);

/** The card a chat belongs to, as primary or member. */
export const cardWith = (state: BoardState, chatId: string) =>
	state.cards.find((card) => card.members.some((m) => m.id === chatId));

const columnNames = (state: BoardState) => state.columns.map((c) => c.name);

const columnCards = (state: BoardState, name: string) =>
	state.columns.find((c) => c.name === name)?.cards ?? [];

// Placement key for the slot before `beforeCardId` in `column`, ignoring the
// card being moved so its old slot does not count as a neighbour.
const keyForSlot = (
	state: BoardState,
	column: string,
	beforeCardId: string | null,
	movingId: string,
) => {
	const ordered = columnCards(state, column).filter((c) => c.id !== movingId);
	const index = beforeCardId
		? ordered.findIndex((c) => c.id === beforeCardId)
		: ordered.length;
	const above = index > 0 ? ordered[index - 1] : undefined;
	const below = index >= 0 ? ordered[index] : undefined;
	return keyBetween(
		above && placementKey(above.primary),
		below && placementKey(below.primary),
	);
};

const relabelColumn = (cards: readonly BoardCard[], column: string): Write[] =>
	cards.flatMap((card) =>
		card.members.map((member) => ({
			chat: member,
			labels: setColumnLabel(member.labels, column),
		})),
	);

/** Moves a card to `column`, before `beforeCardId` or at the end. Own slot is a no-op. */
export const moveCard = (
	state: BoardState,
	cardId: string,
	column: string,
	beforeCardId: string | null,
): Plan | null => {
	const card = cardOf(state, cardId);
	if (!card) return null;
	const ordered = columnCards(state, column);
	const index = ordered.findIndex((c) => c.id === cardId);
	const nextId = ordered[index + 1]?.id ?? null;
	if (index >= 0 && (beforeCardId === cardId || beforeCardId === nextId)) {
		return null;
	}
	const placedAt = keyForSlot(state, column, beforeCardId, cardId);
	return {
		writes: card.members.map((member) => ({
			chat: member,
			labels:
				member.id === card.id
					? setPositionLabel(setColumnLabel(member.labels, column), placedAt)
					: setColumnLabel(member.labels, column),
		})),
	};
};

// The side that keeps its primary: an existing group beats a single chat,
// then a titled card beats an untitled one, then the drop target. So a
// group dropped onto a lone chat absorbs it instead of losing its title.
const mergeKeeper = (source: BoardCard, target: BoardCard): BoardCard => {
	const grouped = (card: BoardCard) => card.members.length > 1;
	if (grouped(source) !== grouped(target)) {
		return grouped(source) ? source : target;
	}
	const titled = (card: BoardCard) => getTitleLabel(card.primary) !== undefined;
	if (titled(source) !== titled(target)) {
		return titled(source) ? source : target;
	}
	return target;
};

// The kept primary moves into the drop target's slot and absorbs the other
// card's notes; a title that would otherwise vanish becomes a note.
export const mergeCards = (
	state: BoardState,
	sourceCardId: string,
	targetCardId: string,
): Plan | null => {
	const source = cardOf(state, sourceCardId);
	const target = cardOf(state, targetCardId);
	if (!source || !target || source === target) return null;
	const keep = mergeKeeper(source, target);
	const join = keep === source ? target : source;
	let labels = setPositionLabel(
		setColumnLabel(keep.primary.labels, target.column),
		placementKey(target.primary),
	);
	if (!keep.color && join.color) labels = setColorLabel(labels, join.color);
	labels = setEffortsLabels(labels, [...keep.efforts, ...join.efforts]);
	let index = nextCommentIndex(keep.comments);
	const carried = join.comments.map((c) => [c.text, c.timestamp] as const);
	if (getTitleLabel(join.primary) && getTitleLabel(keep.primary)) {
		carried.push([`Merged card: ${join.title}`, Date.now()]);
	}
	for (const [text, timestamp] of carried) {
		Object.assign(labels, commentLabels(index, text, timestamp));
		index += 1;
	}
	return {
		writes: [
			{ chat: keep.primary, labels },
			...join.members.map((member) => ({
				chat: member,
				labels: setColumnLabel(
					setGroupLabel(stripCardLabels(member.labels), keep.id, member.id),
					target.column,
				),
			})),
			...(keep === target
				? []
				: keep.members
						.filter((member) => member.id !== keep.id)
						.map((member) => ({
							chat: member,
							labels: setColumnLabel(member.labels, target.column),
						}))),
		],
		undo: `Merged into "${keep.title}"`,
	};
};

// A primary that leaves hands the card (title, color, position, notes) to
// the oldest remaining member and the others follow it. The card keeps its
// effective title, so the departure renames nothing.
const leaveGroup = (chat: Chat, card: BoardCard): Write[] => {
	if (chat.id !== card.id || card.members.length < 2) return [];
	const [next, ...rest] = card.members.filter((m) => m.id !== chat.id);
	if (!next) return [];
	return [
		{
			chat: next,
			labels: setTitleLabel(
				{
					...setGroupLabel(next.labels, next.id, next.id),
					...takeCardLabels(card.primary.labels),
				},
				card.title,
			),
		},
		...rest.map((member) => ({
			chat: member,
			labels: setGroupLabel(member.labels, next.id, member.id),
		})),
	];
};

const detachAt = (
	chat: Chat,
	card: BoardCard,
	column: string,
	placedAt: number,
): Plan => ({
	writes: [
		...leaveGroup(chat, card),
		{
			chat,
			labels: setPositionLabel(
				setColumnLabel(stripCardLabels(chat.labels), column),
				placedAt,
			),
		},
	],
	undo: `Removed "${chat.title}" from "${card.title}"`,
});

const memberOf = (state: BoardState, chatId: string) => {
	const card = cardWith(state, chatId);
	const chat = card?.members.find((m) => m.id === chatId);
	return card && chat ? { card, chat } : undefined;
};

/** Makes the chat its own card in `column` at the slot, whatever its role in its card. */
export const detachChat = (
	state: BoardState,
	chatId: string,
	column: string,
	beforeCardId: string | null,
): Plan | null => {
	const found = memberOf(state, chatId);
	if (!found) return null;
	return detachAt(
		found.chat,
		found.card,
		column,
		keyForSlot(state, column, beforeCardId, chatId),
	);
};

/** From the row menu: the chat becomes its own card right under the one it left. */
export const removeFromGroup = (
	state: BoardState,
	chatId: string,
): Plan | null => {
	const found = memberOf(state, chatId);
	if (!found) return null;
	const { card, chat } = found;
	return detachAt(chat, card, card.column, placementKey(card.primary) - 1);
};

/** Moves one chat out of its card into the target card. */
export const joinCard = (
	state: BoardState,
	chatId: string,
	targetCardId: string,
): Plan | null => {
	const found = memberOf(state, chatId);
	const target = cardOf(state, targetCardId);
	if (!found || !target || found.card === target) return null;
	const { card, chat } = found;
	return {
		writes: [
			...leaveGroup(chat, card),
			{
				chat,
				labels: setColumnLabel(
					setGroupLabel(stripCardLabels(chat.labels), target.id, chat.id),
					target.column,
				),
			},
		],
		undo: `Added "${chat.title}" to "${target.title}"`,
	};
};

export const moveColumn = (
	state: BoardState,
	name: string,
	target: ColumnSlot,
): Plan | null => {
	const names = columnNames(state);
	if (
		name === target.name ||
		!names.includes(name) ||
		!names.includes(target.name)
	) {
		return null;
	}
	const order = names.filter((n) => n !== name);
	order.splice(
		order.indexOf(target.name) + (target.side === "after" ? 1 : 0),
		0,
		name,
	);
	return { writes: [], storage: { columnOrder: order } };
};

export const addColumn = (state: BoardState, name: string): Plan | null => {
	const names = columnNames(state);
	if (names.includes(name)) return null;
	return {
		writes: [],
		storage: {
			emptyColumns: [...state.storage.emptyColumns, name],
			columnOrder: [...names, name],
		},
	};
};

// Renames keep every card where it was; only the column name changes.
export const renameColumn = (
	state: BoardState,
	from: string,
	to: string,
): Plan | null => {
	const names = columnNames(state);
	if (from === INBOX_COLUMN || !names.includes(from) || names.includes(to)) {
		return null;
	}
	const rename = (n: string) => (n === from ? to : n);
	return {
		writes: relabelColumn(columnCards(state, from), to),
		storage: {
			emptyColumns: state.storage.emptyColumns.map(rename),
			columnOrder: state.storage.columnOrder.map(rename),
		},
	};
};

/** Deletes a column; its cards go back to Inbox. */
export const deleteColumn = (state: BoardState, name: string): Plan | null => {
	if (name === INBOX_COLUMN || !columnNames(state).includes(name)) return null;
	return {
		writes: relabelColumn(columnCards(state, name), INBOX_COLUMN),
		storage: {
			emptyColumns: state.storage.emptyColumns.filter((n) => n !== name),
			columnOrder: state.storage.columnOrder.filter((n) => n !== name),
		},
	};
};

const primaryWrite = (
	state: BoardState,
	cardId: string,
	labels: (card: BoardCard) => Record<string, string>,
): Plan | null => {
	const card = cardOf(state, cardId);
	return card
		? { writes: [{ chat: card.primary, labels: labels(card) }] }
		: null;
};

export const setCardColor = (
	state: BoardState,
	cardId: string,
	color: CardColor | undefined,
): Plan | null =>
	primaryWrite(state, cardId, (card) =>
		setColorLabel(card.primary.labels, color),
	);

export const setCardEfforts = (
	state: BoardState,
	cardId: string,
	names: readonly string[],
): Plan | null =>
	primaryWrite(state, cardId, (card) =>
		setEffortsLabels(card.primary.labels, names),
	);

/** An effort on the board and how many cards carry it. */
export type EffortCount = Readonly<{ name: string; count: number }>;

/** Every effort on the board with its card count, in order of first appearance. */
export const effortsOf = (cards: readonly BoardCard[]): EffortCount[] => {
	const counts = new Map<string, number>();
	for (const card of cards) {
		for (const name of card.efforts) {
			counts.set(name, (counts.get(name) ?? 0) + 1);
		}
	}
	return [...counts].map(([name, count]) => ({ name, count }));
};

// A single chat's card title is the chat title, so renaming the card renames
// the chat. A group has its own title label.
export const renameCard = (
	state: BoardState,
	cardId: string,
	title: string,
): Plan | null => {
	const card = cardOf(state, cardId);
	if (!card) return null;
	if (card.members.length === 1) {
		return { writes: [], titles: [{ chat: card.primary, title }] };
	}
	return {
		writes: [
			{ chat: card.primary, labels: setTitleLabel(card.primary.labels, title) },
		],
	};
};

export const renameChat = (
	state: BoardState,
	chatId: string,
	title: string,
): Plan | null => {
	const found = memberOf(state, chatId);
	return found ? { writes: [], titles: [{ chat: found.chat, title }] } : null;
};

export const addNote = (
	state: BoardState,
	cardId: string,
	text: string,
): Plan | null =>
	primaryWrite(state, cardId, (card) =>
		addCommentLabels(card.primary.labels, text),
	);

export const editNote = (
	state: BoardState,
	cardId: string,
	index: number,
	text: string,
): Plan | null =>
	primaryWrite(state, cardId, (card) =>
		updateCommentLabels(card.primary.labels, index, text),
	);

export const removeNote = (
	state: BoardState,
	cardId: string,
	index: number,
): Plan | null =>
	primaryWrite(state, cardId, (card) =>
		removeCommentLabels(card.primary.labels, index),
	);

// Notes are renumbered on both cards so index stays the display order.
// Useful after a merge or ungroup, when a note belongs with another card.
export const moveNote = (
	state: BoardState,
	fromCardId: string,
	noteIndex: number,
	toCardId: string,
	slot: NoteSlot | null,
): Plan | null => {
	const from = cardOf(state, fromCardId);
	const to = cardOf(state, toCardId);
	const note = from?.comments.find((c) => c.index === noteIndex);
	if (!from || !to || !note) return null;
	const same = from === to;
	const remaining = from.comments.filter((c) => c.index !== note.index);
	const list = [...(same ? remaining : to.comments)];
	const at = slot
		? list.findIndex((c) => c.index === slot.index) +
			(slot.side === "after" ? 1 : 0)
		: list.length;
	list.splice(at < 0 ? list.length : at, 0, note);
	return {
		writes: [
			{ chat: to.primary, labels: setCommentsLabels(to.primary.labels, list) },
			...(same
				? []
				: [
						{
							chat: from.primary,
							labels: setCommentsLabels(from.primary.labels, remaining),
						},
					]),
		],
		undo: same ? "Moved note" : `Moved note to "${to.title}"`,
	};
};

/**
 * Labels for a chat created from the board, sent with the create request so
 * the chat is born in place and no label write can race the list refetch.
 * Null for an unknown card.
 */
export const newChatLabels = (
	state: BoardState,
	target: DraftTarget,
): Record<string, string> | null => {
	if ("column" in target) {
		const first = columnCards(state, target.column)[0];
		return setPositionLabel(
			setColumnLabel({}, target.column),
			keyBetween(undefined, first && placementKey(first.primary)),
		);
	}
	const card = cardOf(state, target.cardId);
	if (!card) return null;
	// Members carry no position; the card's primary places the group. The
	// chat has no id yet, so nothing can equal the primary here.
	return setColumnLabel(setGroupLabel({}, card.id, ""), card.column);
};

/** The card as text for a new chat's first message, when the user asks for it. */
export const cardContext = (card: BoardCard): string =>
	[
		"Card context",
		`Title: ${card.title}`,
		card.comments.length
			? ["Notes:", ...card.comments.map((note) => `- ${note.text}`)].join("\n")
			: "Notes: none",
		"Chats:",
		...card.members.map(
			(chat) =>
				`- ${chat.title} (${chat.id}) status: ${chat.status}; last turn: ${chat.last_turn_summary ?? "none"}`,
		),
	].join("\n");
