import { useLocation } from "react-router";

export type DeploymentSection = "general" | "infrastructure" | "authentication";

// Route-prefix-to-section mapping. Order matters: first match wins.
const SECTION_ROUTES: Array<[string[], DeploymentSection]> = [
	[
		[
			"/deployment/overview",
			"/deployment/licenses",
			"/deployment/appearance",
			"/deployment/users",
			"/deployment/groups",
			"/deployment/notifications",
			"/deployment/premium",
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
			"/deployment/external-auth",
			"/deployment/oauth2-provider",
			"/deployment/idp-org-sync",
		],
		"authentication",
	],
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
