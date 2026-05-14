import { useLocation } from "react-router";

export type AISection =
	| "ai-governance"
	| "providers"
	| "models"
	| "spend"
	| "agents";

// Route-prefix-to-section mapping. Order matters: first match wins.
const SECTION_ROUTES: Array<[string[], AISection]> = [
	[["/ai/governance"], "ai-governance"],
	[["/ai/providers"], "providers"],
	[["/ai/models"], "models"],
	[["/ai/spend"], "spend"],
	[["/ai/agents"], "agents"],
];

/**
 * Derives the active sidebar section from the current route so the
 * correct item or accordion can be highlighted on navigation.
 */
export function useActiveAISection(): AISection {
	const { pathname } = useLocation();

	for (const [routes, section] of SECTION_ROUTES) {
		for (const route of routes) {
			if (pathname === route || pathname.startsWith(`${route}/`)) {
				return section;
			}
		}
	}

	// Fall back to "agents" for unknown AI routes.
	return "agents";
}
