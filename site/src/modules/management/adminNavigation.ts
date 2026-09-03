import type { BuildInfoResponse, Experiment } from "#/api/typesGenerated";
import { PREMIUM_PAGE_PATH } from "#/components/Paywall/Paywall";
import {
	canAccessAnyChatModelConfig,
	type Permissions,
} from "#/modules/permissions";
import { getPrereleaseFlag } from "#/utils/buildInfo";

export interface AdminNavItem {
	label: string;
	href: string;
	visible: boolean;
	/** Match the route exactly instead of by prefix. */
	end?: boolean;
	/** Route prefix that marks this item active when it differs from href. */
	matchPrefix?: string;
	/**
	 * Additional route prefixes that also mark this item active, for pages
	 * reachable from more than one URL (for example organization-scoped
	 * model pages).
	 */
	activePrefixes?: string[];
	/**
	 * Pages with wide content (large tables) where the sidebar should
	 * settle collapsed so the content gets the full width.
	 */
	wideContent?: boolean;
}

export interface AdminNavSection {
	/** Stable key used to persist the accordion open state. */
	key: string;
	label: string;
	items: AdminNavItem[];
}

const firstVisibleHref = (items: AdminNavItem[]): string | undefined =>
	items.find((item) => item.visible)?.href;

// Deployment

interface DeploymentNavContext {
	permissions: Permissions;
	/** Licensed, non-trial deployments have nothing to upgrade to. */
	hidePremiumTab: boolean;
	experiments: Experiment[];
	buildInfo: BuildInfoResponse;
}

export const deploymentNavSections = ({
	permissions,
	hidePremiumTab,
	experiments,
	buildInfo,
}: DeploymentNavContext): AdminNavSection[] => [
	{
		key: "deployment-general",
		label: "General",
		items: [
			{
				label: "Overview",
				href: "/deployment/overview",
				visible: permissions.viewDeploymentConfig,
			},
			{
				label: "Licenses",
				href: "/deployment/licenses",
				visible: permissions.viewAllLicenses,
			},
			{
				label: "Users",
				href: "/deployment/users",
				visible: permissions.viewAllUsers,
			},
			{
				label: "Appearance",
				href: "/deployment/appearance",
				visible: permissions.editDeploymentConfig,
			},
			{
				label: "Notifications",
				href: "/deployment/notifications",
				visible: permissions.viewNotificationTemplate,
			},
			{
				label: "Trial Upgrade",
				href: PREMIUM_PAGE_PATH,
				visible: !hidePremiumTab,
			},
		],
	},
	{
		key: "deployment-infrastructure",
		label: "Infrastructure",
		items: [
			{
				label: "Security",
				href: "/deployment/security",
				visible: permissions.viewDeploymentConfig,
			},
			{
				label: "Observability",
				href: "/deployment/observability",
				visible: permissions.viewDeploymentConfig,
			},
			{
				label: "Workspace proxies",
				href: "/deployment/workspace-proxies",
				visible: permissions.readWorkspaceProxies,
			},
			{
				label: "Network",
				href: "/deployment/network",
				visible: permissions.viewDeploymentConfig,
			},
		],
	},
	{
		key: "deployment-authentication",
		label: "Authentication",
		items: [
			{
				label: "User authentication",
				href: "/deployment/userauth",
				visible: permissions.viewDeploymentConfig,
			},
			{
				label: "OAuth2 Applications",
				href: "/deployment/oauth2-provider/apps",
				matchPrefix: "/deployment/oauth2-provider",
				visible:
					permissions.viewDeploymentConfig &&
					(experiments.includes("oauth2") ||
						getPrereleaseFlag(buildInfo) === "devel"),
			},
			{
				label: "External Authentication",
				href: "/deployment/external-auth",
				visible: permissions.viewDeploymentConfig,
			},
			{
				label: "IdP Organization sync",
				href: "/deployment/idp-org-sync",
				visible: permissions.viewOrganizationIDPSyncSettings,
			},
		],
	},
];

/**
 * The first deployment page the user can see, walking General, then
 * Infrastructure, then Authentication. Falls back to the users page,
 * which is the historical landing page for users without deployment
 * config access.
 */
export const firstVisibleDeploymentPage = (
	context: DeploymentNavContext,
): string =>
	deploymentNavSections(context)
		.map((section) => firstVisibleHref(section.items))
		.find((href) => href !== undefined) ?? "/deployment/users";

// AI

interface AIAccess {
	/** The user can manage chat models in at least one organization. */
	canAccessOrganizationModels: boolean;
	/** The user can share MCP servers in at least one organization. */
	canShareOrganizationMCPServers: boolean;
}

export const aiNavItems = (
	permissions: Permissions,
	{ canAccessOrganizationModels }: AIAccess,
): AdminNavItem[] => [
	{
		label: "AI Governance",
		href: "/ai/settings/governance",
		visible: permissions.viewDeploymentConfig,
	},
	{
		label: "AI Gateway keys",
		href: "/ai/settings/gateway-keys",
		visible: permissions.viewAIGatewayKeys,
	},
	{
		label: "Providers",
		href: "/ai/settings/providers",
		visible: permissions.viewAnyAIProvider,
	},
	{
		label: "Models",
		href: "/ai/settings/models",
		visible:
			canAccessAnyChatModelConfig(permissions) || canAccessOrganizationModels,
		// Organization-scoped model pages live under a different prefix.
		activePrefixes: ["/ai/settings/models", "/ai/settings/organizations"],
	},
];

const CODER_AGENTS_PREFIXES = [
	"/ai/settings/coder-agents",
	"/ai/settings/mcp-servers",
	"/ai/settings/templates",
	"/ai/settings/instructions",
	"/ai/settings/lifecycle",
];

export const aiCoderAgentsSection = (
	permissions: Permissions,
	{ canAccessOrganizationModels, canShareOrganizationMCPServers }: AIAccess,
): AdminNavSection => {
	const canListMCPServers =
		permissions.editDeploymentConfig ||
		permissions.viewAnyMCPServerConfigs ||
		permissions.updateAnyMCPServerConfig ||
		permissions.deleteAnyMCPServerConfig ||
		canShareOrganizationMCPServers;
	return {
		key: "ai-coder-agents",
		label: "Coder Agents",
		items: [
			{
				label: "General",
				href: "/ai/settings/coder-agents",
				end: true,
				visible:
					permissions.editDeploymentConfig || canAccessOrganizationModels,
			},
			{
				label: "MCP servers",
				// Users who can only create servers land on the add form.
				href: canListMCPServers
					? "/ai/settings/mcp-servers"
					: "/ai/settings/mcp-servers/add",
				matchPrefix: "/ai/settings/mcp-servers",
				visible: canListMCPServers || permissions.createAnyMCPServerConfig,
			},
			{
				label: "Templates",
				href: "/ai/settings/templates",
				visible: permissions.updateAnyTemplate,
			},
			{
				label: "Instructions",
				href: "/ai/settings/instructions",
				visible: permissions.editDeploymentConfig,
			},
			{
				label: "Lifecycle",
				href: "/ai/settings/lifecycle",
				visible: permissions.editDeploymentConfig,
			},
		],
	};
};

/**
 * Whether the AI section is shown. Mirrors the navbar's admin menu gate:
 * any site-wide AI permission, or organization-scoped model or MCP
 * sharing access.
 */
export const canViewAISettings = (
	permissions: Permissions,
	{ canAccessOrganizationModels, canShareOrganizationMCPServers }: AIAccess,
): boolean =>
	permissions.viewAnyAIProvider ||
	permissions.viewAIGatewayKeys ||
	permissions.editDeploymentConfig ||
	permissions.viewAnyMCPServerConfigs ||
	permissions.createAnyMCPServerConfig ||
	permissions.updateAnyMCPServerConfig ||
	permissions.deleteAnyMCPServerConfig ||
	permissions.updateAnyTemplate ||
	canAccessAnyChatModelConfig(permissions) ||
	canAccessOrganizationModels ||
	canShareOrganizationMCPServers;

// Logs

interface LogsVisibility {
	canViewAuditLog: boolean;
	canViewConnectionLog: boolean;
	canViewAIBridge: boolean;
}

export const logsNavItems = ({
	canViewAuditLog,
	canViewConnectionLog,
	canViewAIBridge,
}: LogsVisibility): AdminNavItem[] => [
	{
		label: "Audit logs",
		href: "/audit",
		visible: canViewAuditLog,
		wideContent: true,
	},
	{
		label: "Connection logs",
		href: "/connectionlog",
		visible: canViewConnectionLog,
		wideContent: true,
	},
	{
		label: "AI session logs",
		href: "/ai-gateway/sessions",
		visible: canViewAIBridge,
		wideContent: true,
	},
];

/**
 * Pages whose content needs the full width, so the sidebar peeks and then
 * collapses on entry. Healthcheck has its own internal navigation, so it
 * behaves the same way.
 */
const wideContentHrefs = (): string[] => [
	...logsNavItems({
		canViewAuditLog: true,
		canViewConnectionLog: true,
		canViewAIBridge: true,
	})
		.filter((item) => item.wideContent)
		.map((item) => item.matchPrefix ?? item.href),
	"/health",
];

/**
 * Whether the route belongs to a page flagged as wide content, including
 * its detail sub-pages (for example individual AI sessions).
 */
export const isWideContentRoute = (pathname: string): boolean =>
	wideContentHrefs().some((href) => isRouteActive(pathname, href));

/**
 * The first log page the user can see: audit, then connection, then AI
 * sessions. Falls back to AI sessions so the redirect always resolves.
 */
export const firstVisibleLogsPage = (visibility: LogsVisibility): string =>
	firstVisibleHref(logsNavItems(visibility)) ?? "/ai-gateway/sessions";

// Route to accordion chain

const isRouteActive = (pathname: string, href: string, end = false) =>
	end
		? pathname === href
		: pathname === href || pathname.startsWith(`${href}/`);

/**
 * Accordion keys that must be open for the current route to be visible
 * in the sidebar, outermost first. Empty for routes outside the admin
 * sections.
 */
export const adminSectionChainForRoute = (
	pathname: string,
	deploymentSections: AdminNavSection[],
): string[] => {
	if (isRouteActive(pathname, "/deployment")) {
		const section = deploymentSections.find((candidate) =>
			candidate.items.some((item) =>
				isRouteActive(pathname, item.matchPrefix ?? item.href),
			),
		);
		return section ? ["deployment", section.key] : ["deployment"];
	}
	if (isRouteActive(pathname, "/organizations")) {
		const isProvisionerPage = /^\/organizations\/[^/]+\/provisioner/.test(
			pathname,
		);
		return isProvisionerPage
			? ["organizations", "organizations-provisioners"]
			: ["organizations"];
	}
	if (isRouteActive(pathname, "/ai/settings")) {
		const isCoderAgentsPage = CODER_AGENTS_PREFIXES.some((prefix) =>
			isRouteActive(pathname, prefix),
		);
		return isCoderAgentsPage ? ["ai", "ai-coder-agents"] : ["ai"];
	}
	if (
		isRouteActive(pathname, "/logs") ||
		isRouteActive(pathname, "/audit") ||
		isRouteActive(pathname, "/connectionlog") ||
		isRouteActive(pathname, "/ai-gateway")
	) {
		return ["logs"];
	}
	return [];
};
