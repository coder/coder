// Keyed by user: accounts sharing a browser must not see each other's
// columns and windows.
const storageKey = (userId: string) => `agents.board.${userId}`;

/** Where a new chat is born: at the top of a column, or as a member of a card. */
export type DraftTarget =
	| Readonly<{ column: string }>
	| Readonly<{ cardId: string }>;

type WindowFrame = Readonly<{
	x: number;
	y: number;
	width: number;
	height: number;
}>;

/**
 * A floating window over the board, in viewport pixels. A chat window shows
 * an existing chat; unpinned ones are hover previews, which live in the same
 * list so pinning does not remount them, but do not survive a reload. A
 * draft window holds the create form for a chat that does not exist yet; it
 * is always pinned and never stored, because the form keeps its text in one
 * shared localStorage draft, so there is at most one draft window.
 */
export type ChatWindow = WindowFrame &
	(
		| Readonly<{ kind: "chat"; chatId: string; pinned: boolean }>
		| Readonly<{
				kind: "draft";
				target: DraftTarget;
				withContext: boolean;
				pinned: true;
		  }>
	);

/** The identity a window keeps across geometry changes and pinning. */
export const windowKey = (win: ChatWindow): string =>
	win.kind === "chat" ? win.chatId : "draft";

export type BoardStorage = Readonly<{
	columnOrder: readonly string[];
	/**
	 * Columns the user created. A label holds a column only while a card is in
	 * it; this keeps a created column on the board while it is empty.
	 */
	emptyColumns: readonly string[];
	/** Chat windows, back to front. */
	windows: readonly ChatWindow[];
	/** The effort whose cards are shown; null shows every card. */
	effortFilter: string | null;
}>;

const DEFAULT_STORAGE: BoardStorage = {
	columnOrder: [],
	emptyColumns: [],
	windows: [],
	effortFilter: null,
};

const isStringArray = (value: unknown): value is string[] =>
	Array.isArray(value) && value.every((v) => typeof v === "string");

const isFiniteNumber = (value: unknown): value is number =>
	typeof value === "number" && Number.isFinite(value);

// Stored windows predate `kind`; a chat id alone identifies a chat window.
const readChatWindow = (value: unknown): ChatWindow | undefined => {
	if (typeof value !== "object" || value === null) return undefined;
	const obj = value as Record<string, unknown>;
	if (
		typeof obj.chatId !== "string" ||
		!isFiniteNumber(obj.x) ||
		!isFiniteNumber(obj.y) ||
		!isFiniteNumber(obj.width) ||
		!isFiniteNumber(obj.height) ||
		obj.pinned !== true
	) {
		return undefined;
	}
	return {
		kind: "chat",
		chatId: obj.chatId,
		x: obj.x,
		y: obj.y,
		width: obj.width,
		height: obj.height,
		pinned: true,
	};
};

/** The stored board state, or defaults when absent or unreadable. Previews are not restored. */
export const readBoardStorage = (userId: string): BoardStorage => {
	const raw = localStorage.getItem(storageKey(userId));
	if (!raw) return DEFAULT_STORAGE;
	try {
		const parsed: unknown = JSON.parse(raw);
		if (typeof parsed !== "object" || parsed === null) return DEFAULT_STORAGE;
		const obj = parsed as Record<string, unknown>;
		return {
			columnOrder: isStringArray(obj.columnOrder) ? obj.columnOrder : [],
			emptyColumns: isStringArray(obj.emptyColumns) ? obj.emptyColumns : [],
			windows: Array.isArray(obj.windows)
				? obj.windows.flatMap((w) => readChatWindow(w) ?? [])
				: [],
			effortFilter:
				typeof obj.effortFilter === "string" ? obj.effortFilter : null,
		};
	} catch {
		return DEFAULT_STORAGE;
	}
};

export const saveBoardStorage = (userId: string, next: BoardStorage): void => {
	localStorage.setItem(storageKey(userId), JSON.stringify(next));
};
