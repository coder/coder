// Keyed by user: accounts sharing a browser must not see each other's
// columns and windows.
const storageKey = (userId: string) => `agents.board.${userId}`;

/**
 * A floating chat window, in viewport pixels. Unpinned windows are hover
 * previews; they live in the same list so pinning does not remount them,
 * but they do not survive a reload.
 */
export type ChatWindow = Readonly<{
	chatId: string;
	x: number;
	y: number;
	width: number;
	height: number;
	pinned: boolean;
}>;

export type BoardStorage = Readonly<{
	columnOrder: readonly string[];
	/** Columns the user created that have no cards yet. Labels cannot hold these. */
	emptyColumns: readonly string[];
	/** Chat windows, back to front. */
	windows: readonly ChatWindow[];
}>;

const DEFAULT_STORAGE: BoardStorage = {
	columnOrder: [],
	emptyColumns: [],
	windows: [],
};

const isStringArray = (value: unknown): value is string[] =>
	Array.isArray(value) && value.every((v) => typeof v === "string");

const isFiniteNumber = (value: unknown): value is number =>
	typeof value === "number" && Number.isFinite(value);

const isWindow = (value: unknown): value is ChatWindow => {
	if (typeof value !== "object" || value === null) return false;
	const obj = value as Record<string, unknown>;
	return (
		typeof obj.chatId === "string" &&
		isFiniteNumber(obj.x) &&
		isFiniteNumber(obj.y) &&
		isFiniteNumber(obj.width) &&
		isFiniteNumber(obj.height) &&
		typeof obj.pinned === "boolean"
	);
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
				? obj.windows.filter(isWindow).filter((w) => w.pinned)
				: [],
		};
	} catch {
		return DEFAULT_STORAGE;
	}
};

export const saveBoardStorage = (userId: string, next: BoardStorage): void => {
	localStorage.setItem(storageKey(userId), JSON.stringify(next));
};
