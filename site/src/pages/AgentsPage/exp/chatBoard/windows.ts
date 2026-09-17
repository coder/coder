import type { ChatWindow } from "./boardStorage";

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
	pinned: boolean,
): ChatWindow => {
	const { width, height } = fittedSize();
	const fitsRight =
		anchor.right + ANCHOR_GAP_PX + width <= window.innerWidth - MARGIN;
	const x = fitsRight
		? anchor.right + ANCHOR_GAP_PX
		: anchor.left - ANCHOR_GAP_PX - width;
	return clampWindow({ chatId, x, y: anchor.top, width, height, pinned });
};

/** A pinned window in the middle of the viewport, for chats opened without a card in view. */
export const windowCentered = (chatId: string): ChatWindow => {
	const { width, height } = fittedSize();
	return {
		chatId,
		x: (window.innerWidth - width) / 2,
		y: (window.innerHeight - height) / 2,
		width,
		height,
		pinned: true,
	};
};

// The list is back to front: the last window is the frontmost. One list
// holds pinned windows and the hover preview (pinned: false), so pinning is
// a flag flip on the same element and a gesture in progress survives it.

/** The preview, if one is showing. */
export const previewOf = (list: readonly ChatWindow[]) =>
	list.find((w) => !w.pinned);

export const dropPreview = (list: readonly ChatWindow[]) =>
	list.filter((w) => w.pinned);

/** Pins `win` and moves it to the front, replacing any window for the same chat. */
export const toFront = (list: readonly ChatWindow[], win: ChatWindow) => [
	...list.filter((w) => w.chatId !== win.chatId),
	{ ...win, pinned: true },
];

/** Raises the window for `chatId`, pinning a preview; a chat without a window is left alone. */
export const raise = (list: readonly ChatWindow[], chatId: string) => {
	const win = list.find((w) => w.chatId === chatId);
	return win ? toFront(list, win) : list;
};

/** New geometry for one window; pinning is not a geometry change and stays as it was. */
export const changeWindow = (list: readonly ChatWindow[], next: ChatWindow) =>
	list.map((w) =>
		w.chatId === next.chatId ? { ...next, pinned: w.pinned } : w,
	);

export const closeWindow = (list: readonly ChatWindow[], chatId: string) =>
	list.filter((w) => w.chatId !== chatId);

/** Escape: the preview goes first, else the frontmost window. */
export const dismissTop = (list: readonly ChatWindow[]) =>
	list.some((w) => !w.pinned) ? dropPreview(list) : list.slice(0, -1);
