import { useCallback, useEffect, useState } from "react";

function readOpenSections(key: string): Set<string> | undefined {
	try {
		const raw = localStorage.getItem(key);
		if (!raw) {
			return undefined;
		}
		const parsed: unknown = JSON.parse(raw);
		if (!Array.isArray(parsed)) {
			return undefined;
		}
		return new Set(
			parsed.filter((value): value is string => typeof value === "string"),
		);
	} catch {
		return undefined;
	}
}

function persistOpenSections(key: string, sections: Set<string>): void {
	try {
		localStorage.setItem(key, JSON.stringify([...sections]));
	} catch {
		// Silently ignore write failures.
	}
}

/**
 * Tracks which sidebar accordions are open, persisted under storageKey.
 * State is fully manual; the only automatic change is opening the chain
 * of sections that contains the current route so the active link is
 * always visible. Nothing is ever closed automatically. defaultOpen is
 * used when nothing has been persisted yet.
 */
export function useOpenSections(
	storageKey: string,
	activeChain: string[],
	defaultOpen: string[],
) {
	const [openSections, setOpenSections] = useState<Set<string>>(
		() => readOpenSections(storageKey) ?? new Set(defaultOpen),
	);

	const chainKey = activeChain.join(",");
	useEffect(() => {
		const chain = chainKey ? chainKey.split(",") : [];
		if (chain.length === 0) {
			return;
		}
		setOpenSections((prev) => {
			if (chain.every((key) => prev.has(key))) {
				return prev;
			}
			const next = new Set(prev);
			for (const key of chain) {
				next.add(key);
			}
			persistOpenSections(storageKey, next);
			return next;
		});
	}, [chainKey, storageKey]);

	const toggleSection = useCallback(
		(key: string) => {
			setOpenSections((prev) => {
				const next = new Set(prev);
				if (next.has(key)) {
					next.delete(key);
				} else {
					next.add(key);
				}
				persistOpenSections(storageKey, next);
				return next;
			});
		},
		[storageKey],
	);

	return { openSections, toggleSection };
}
