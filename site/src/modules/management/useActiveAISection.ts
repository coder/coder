import { useLocation } from "react-router";

export type AISection =
	| "governance"
	| "gateway-keys"
	| "providers"
	| "coder-agents";

// Route-prefix-to-section mapping. Order matters: first match wins.
const SECTION_ROUTES: Array<[string[], AISection]> = [
	[["/ai/settings/governance"], "governance"],
	[["/ai/settings/gateway-keys"], "gateway-keys"],
	[["/ai/settings/providers"], "providers"],
	[
		[
			"/ai/settings/coder-agents",
			"/ai/settings/models",
			"/ai/settings/mcp-servers",
			"/ai/settings/templates",
			"/ai/settings/spend",
			"/ai/settings/instructions",
			"/ai/settings/lifecycle",
		],
		"coder-agents",
	],
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

	// Fall back to "coder-agents" for unknown AI settings routes.
	return "coder-agents";
}
