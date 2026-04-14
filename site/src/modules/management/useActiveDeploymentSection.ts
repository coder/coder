import { useLocation } from "react-router";

export type DeploymentSection =
	| "general"
	| "infrastructure"
	| "authentication"
	| "ai-settings"
	| "ai-governance";

// Route-prefix-to-section mapping. Order matters: first match wins.
// Longer prefixes must precede shorter ones to avoid false matches.
const SECTION_ROUTES: Array<[string[], DeploymentSection]> = [
	[
		[
			"/deployment/overview",
			"/deployment/appearance",
			"/deployment/notifications",
			"/deployment/users",
			"/deployment/licenses",
			"/deployment/premium",
			"/deployment/groups",
		],
		"general",
	],
	[
		[
			"/deployment/security",
			"/deployment/observability",
			"/deployment/workspace-proxies",
			"/deployment/network",
		],
		"infrastructure",
	],
	[
		[
			"/deployment/userauth",
			"/deployment/oauth2-provider",
			"/deployment/external-auth",
			"/deployment/idp-org-sync",
		],
		"authentication",
	],
	[["/deployment/ai-settings"], "ai-settings"],
	[["/deployment/ai-governance"], "ai-governance"],
];

/**
 * Derives the active sidebar section from the current route so the
 * correct accordion can be opened on navigation.
 */
export function useActiveDeploymentSection(): DeploymentSection {
	const { pathname } = useLocation();

	for (const [routes, section] of SECTION_ROUTES) {
		for (const route of routes) {
			if (pathname === route || pathname.startsWith(`${route}/`)) {
				return section;
			}
		}
	}

	// Fall back to "general" for unknown deployment routes.
	return "general";
}
