import { useCallback, useState } from "react";

const STORAGE_KEY = "agents.board";

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

const readStorage = (): BoardStorage => {
	const raw = localStorage.getItem(STORAGE_KEY);
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

export const useBoardStorage = () => {
	const [storage, setStorage] = useState<BoardStorage>(readStorage);
	const update = useCallback(
		(
			patch:
				| Partial<BoardStorage>
				| ((prev: BoardStorage) => Partial<BoardStorage>),
		) => {
			setStorage((prev) => {
				const next = {
					...prev,
					...(typeof patch === "function" ? patch(prev) : patch),
				};
				localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
				return next;
			});
		},
		[],
	);
	return [storage, update] as const;
};
