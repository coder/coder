import { useCallback, useState } from "react";

const STORAGE_KEY = "agents.board";

type BoardStorage = Readonly<{
	columnOrder: readonly string[];
	/** Columns the user created that have no cards yet. Labels cannot hold these. */
	emptyColumns: readonly string[];
	/** Fraction of the pane height given to the board when a chat is open below. */
	splitRatio: number;
}>;

const DEFAULT_STORAGE: BoardStorage = {
	columnOrder: [],
	emptyColumns: [],
	splitRatio: 0.4,
};

const isStringArray = (value: unknown): value is string[] =>
	Array.isArray(value) && value.every((v) => typeof v === "string");

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
			splitRatio:
				typeof obj.splitRatio === "number" && Number.isFinite(obj.splitRatio)
					? obj.splitRatio
					: DEFAULT_STORAGE.splitRatio,
		};
	} catch {
		return DEFAULT_STORAGE;
	}
};

export const useBoardStorage = () => {
	const [storage, setStorage] = useState<BoardStorage>(readStorage);
	const update = useCallback((patch: Partial<BoardStorage>) => {
		setStorage((prev) => {
			const next = { ...prev, ...patch };
			localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
			return next;
		});
	}, []);
	return [storage, update] as const;
};
