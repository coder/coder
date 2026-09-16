import { useCallback, useState } from "react";

const STORAGE_KEY = "agents.board";

export type ChatPane = Readonly<{
	tabs: readonly string[];
	active: string;
}>;

type BoardStorage = Readonly<{
	columnOrder: readonly string[];
	/** Columns the user created that have no cards yet. Labels cannot hold these. */
	emptyColumns: readonly string[];
	/** Fraction of the pane height given to the board when a chat is open below. */
	splitRatio: number;
	/** Open chat tabs below the board, at most two panes side by side. */
	panes: readonly ChatPane[];
	focusedPane: number;
	/** Chats hidden but kept; a control in the board header restores them. */
	chatsCollapsed: boolean;
}>;

const DEFAULT_STORAGE: BoardStorage = {
	columnOrder: [],
	emptyColumns: [],
	splitRatio: 0.4,
	panes: [],
	focusedPane: 0,
	chatsCollapsed: false,
};

const isStringArray = (value: unknown): value is string[] =>
	Array.isArray(value) && value.every((v) => typeof v === "string");

const isPane = (value: unknown): value is ChatPane => {
	if (typeof value !== "object" || value === null) return false;
	const obj = value as Record<string, unknown>;
	return (
		isStringArray(obj.tabs) &&
		typeof obj.active === "string" &&
		obj.tabs.includes(obj.active)
	);
};

const readStorage = (): BoardStorage => {
	const raw = localStorage.getItem(STORAGE_KEY);
	if (!raw) return DEFAULT_STORAGE;
	try {
		const parsed: unknown = JSON.parse(raw);
		if (typeof parsed !== "object" || parsed === null) return DEFAULT_STORAGE;
		const obj = parsed as Record<string, unknown>;
		const panes = Array.isArray(obj.panes)
			? obj.panes.filter(isPane).slice(0, 2)
			: [];
		return {
			columnOrder: isStringArray(obj.columnOrder) ? obj.columnOrder : [],
			emptyColumns: isStringArray(obj.emptyColumns) ? obj.emptyColumns : [],
			splitRatio:
				typeof obj.splitRatio === "number" && Number.isFinite(obj.splitRatio)
					? obj.splitRatio
					: DEFAULT_STORAGE.splitRatio,
			panes,
			focusedPane:
				typeof obj.focusedPane === "number" && obj.focusedPane < panes.length
					? obj.focusedPane
					: 0,
			chatsCollapsed: obj.chatsCollapsed === true,
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
