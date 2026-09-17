import { type ChatWindow, type DraftTarget, windowKey } from "./boardStorage";

const DEFAULT_SIZE = { width: 520, height: 640 };
export const MIN_WINDOW_SIZE = { width: 320, height: 240 };
/** Space kept between a window and the viewport edge. */
const MARGIN = 12;
/** Space between a window and the card element it opens beside. */
const ANCHOR_GAP_PX = 8;

// The default size, shrunk on small viewports so the margin survives.
const fittedSize = () => ({
	width: Math.min(DEFAULT_SIZE.width, window.innerWidth - 2 * MARGIN),
	height: Math.min(DEFAULT_SIZE.height, window.innerHeight - 2 * MARGIN),
});

export const clampWindow = (w: ChatWindow): ChatWindow => ({
	...w,
	x: Math.max(MARGIN, Math.min(w.x, window.innerWidth - w.width - MARGIN)),
	y: Math.max(MARGIN, Math.min(w.y, window.innerHeight - w.height - MARGIN)),
});

/** A window beside `anchor`, to its right when there is room, kept on screen. */
export const windowBeside = (
	chatId: string,
	anchor: DOMRect,
	{ pinned }: Readonly<{ pinned: boolean }>,
): ChatWindow => {
	const { width, height } = fittedSize();
	const fitsRight =
		anchor.right + ANCHOR_GAP_PX + width <= window.innerWidth - MARGIN;
	const x = fitsRight
		? anchor.right + ANCHOR_GAP_PX
		: anchor.left - ANCHOR_GAP_PX - width;
	return clampWindow({
		kind: "chat",
		chatId,
		x,
		y: anchor.top,
		width,
		height,
		pinned,
	});
};

const centered = () => {
	const { width, height } = fittedSize();
	return {
		x: (window.innerWidth - width) / 2,
		y: (window.innerHeight - height) / 2,
		width,
		height,
	};
};

/** A pinned window in the middle of the viewport, for chats opened without a card in view. */
export const windowCentered = (chatId: string): ChatWindow => ({
	kind: "chat",
	chatId,
	...centered(),
	pinned: true,
});

/** The create form for a chat that will be born at `target`, centred and pinned. */
export const draftWindow = (
	target: DraftTarget,
): ChatWindow & { kind: "draft" } => ({
	kind: "draft",
	target,
	withContext: false,
	...centered(),
	pinned: true,
});

// The list is back to front: the last window is the frontmost. One list
// holds pinned windows and the hover preview (pinned: false), so pinning is
// a flag flip on the same element and a gesture in progress survives it.

export const dropPreview = (list: readonly ChatWindow[]) =>
	list.filter((w) => w.pinned);

/** Pins `win` and moves it to the front, replacing any window with the same key. */
export const toFront = (
	list: readonly ChatWindow[],
	win: ChatWindow,
): ChatWindow[] => [
	...list.filter((w) => windowKey(w) !== windowKey(win)),
	{ ...win, pinned: true },
];

/** Raises the window for `key`, pinning a preview; a key without a window is left alone. */
export const raise = (list: readonly ChatWindow[], key: string) => {
	const win = list.find((w) => windowKey(w) === key);
	return win ? toFront(list, win) : list;
};

/**
 * New geometry, or a toggled draft option, for one window. Pinning is not a
 * geometry change and stays as it was; drafts are always pinned.
 */
export const changeWindow = (
	list: readonly ChatWindow[],
	next: ChatWindow,
): ChatWindow[] =>
	list.map((w) => {
		if (windowKey(w) !== windowKey(next)) return w;
		return next.kind === "chat" ? { ...next, pinned: w.pinned } : next;
	});

export const closeWindow = (list: readonly ChatWindow[], key: string) =>
	list.filter((w) => windowKey(w) !== key);

/**
 * The chat a draft submitted takes over the draft's frame and stack place.
 * If that draft was replaced or closed while the request ran, the chat
 * still exists, so it opens on its own.
 */
export const draftCreated = (
	list: readonly ChatWindow[],
	target: DraftTarget,
	chatId: string,
): ChatWindow[] => {
	const draft = list.find((w) => w.kind === "draft");
	const sameTarget =
		draft?.kind === "draft" &&
		("column" in target
			? "column" in draft.target && draft.target.column === target.column
			: "cardId" in draft.target && draft.target.cardId === target.cardId);
	if (!sameTarget) return toFront(list, windowCentered(chatId));
	return list.map((w) =>
		w === draft
			? {
					kind: "chat",
					chatId,
					x: w.x,
					y: w.y,
					width: w.width,
					height: w.height,
					pinned: true,
				}
			: w,
	);
};

/** Escape: the preview goes first, else the frontmost window. */
export const dismissTop = (list: readonly ChatWindow[]) =>
	list.some((w) => !w.pinned) ? dropPreview(list) : list.slice(0, -1);
