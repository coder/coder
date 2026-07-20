import { useLocation } from "react-router";

export type LogsSection = "audit" | "connection" | "ai-sessions";

// Route-prefix-to-section mapping. Order matters: first match wins.
const SECTION_ROUTES: Array<[string[], LogsSection]> = [
	[["/audit"], "audit"],
	[["/connectionlog"], "connection"],
	[["/ai-gateway/sessions"], "ai-sessions"],
];

/**
 * Derives the active sidebar section from the current route so the
 * correct item can be highlighted on navigation.
 */
export function useActiveLogsSection(): LogsSection {
	const { pathname } = useLocation();

	for (const [routes, section] of SECTION_ROUTES) {
		for (const route of routes) {
			if (pathname === route || pathname.startsWith(`${route}/`)) {
				return section;
			}
		}
	}

	// Fall back to "audit" for unknown logs routes.
	return "audit";
}
