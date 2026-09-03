import { useCallback, useEffect, useState } from "react";

/**
 * Tracks which sidebar accordions are open for the current page session.
 * On first load only the chain of sections containing the current route
 * is open (falling back to defaultOpen when the route is not in the nav),
 * so every other section starts collapsed. After that the state is fully
 * manual: navigating opens the chain for the new route, but nothing is
 * ever closed automatically. State is not persisted, so a refresh returns
 * to the collapsed default. initialOpen replaces the computed initial
 * state entirely (used by stories to show specific sections open).
 */
export function useOpenSections(
	activeChain: string[],
	defaultOpen: string[],
	initialOpen?: string[],
) {
	const [openSections, setOpenSections] = useState<Set<string>>(
		() =>
			new Set(
				initialOpen ?? (activeChain.length > 0 ? activeChain : defaultOpen),
			),
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
			return next;
		});
	}, [chainKey]);

	const toggleSection = useCallback((key: string) => {
		setOpenSections((prev) => {
			const next = new Set(prev);
			if (next.has(key)) {
				next.delete(key);
			} else {
				next.add(key);
			}
			return next;
		});
	}, []);

	return { openSections, toggleSection };
}
