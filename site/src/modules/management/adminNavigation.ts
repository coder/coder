import type { BuildInfoResponse, Experiment } from "#/api/typesGenerated";
import { linkToAuditing } from "#/modules/navigation";
import type { Permissions } from "#/modules/permissions";
import { getPrereleaseFlag } from "#/utils/buildInfo";

export interface AdminNavItem {
	label: string;
	href: string;
	visible: boolean;
	/** Match the route exactly instead of by prefix. */
	end?: boolean;
	/** Route prefix that marks this item active when it differs from href. */
	matchPrefix?: string;
	/** Marks a link that leaves the current section (shown with an arrow). */
	external?: boolean;
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
	hasPremiumLicense: boolean;
	experiments: Experiment[];
	buildInfo: BuildInfoResponse;
}

export const deploymentNavSections = ({
	permissions,
	hasPremiumLicense,
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
				label: "Groups",
				href: "/deployment/groups",
				visible: permissions.viewAnyGroup,
				external: true,
			},
			{
				label: "Premium",
				href: "/deployment/premium",
				visible: !hasPremiumLicense,
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

export const aiNavItems = (permissions: Permissions): AdminNavItem[] => [
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
		visible: permissions.editDeploymentConfig,
	},
];

const CODER_AGENTS_ITEMS = [
	{ label: "General", href: "/ai/settings/coder-agents", end: true },
	{ label: "MCP servers", href: "/ai/settings/mcp-servers" },
	{ label: "Templates", href: "/ai/settings/templates" },
	{ label: "Spend", href: "/ai/settings/spend" },
	{ label: "Instructions", href: "/ai/settings/instructions" },
	{ label: "Lifecycle", href: "/ai/settings/lifecycle" },
];

export const aiCoderAgentsSection = (
	permissions: Permissions,
): AdminNavSection => ({
	key: "ai-coder-agents",
	label: "Coder Agents",
	items: CODER_AGENTS_ITEMS.map((item) => ({
		...item,
		visible: permissions.editDeploymentConfig,
	})),
});

export const canViewAISettings = (permissions: Permissions): boolean =>
	permissions.viewAnyAIProvider ||
	permissions.viewAIGatewayKeys ||
	permissions.editDeploymentConfig;

/**
 * The first AI settings page the user can see, in sidebar order. Falls
 * back to providers, the historical landing page.
 */
export const firstVisibleAIPage = (permissions: Permissions): string =>
	firstVisibleHref([
		...aiNavItems(permissions),
		...aiCoderAgentsSection(permissions).items,
	]) ?? "/ai/settings/providers";

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
		href: linkToAuditing,
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
		const isCoderAgentsPage = CODER_AGENTS_ITEMS.some((item) =>
			isRouteActive(pathname, item.href),
		);
		return isCoderAgentsPage ? ["ai", "ai-coder-agents"] : ["ai"];
	}
	if (
		isRouteActive(pathname, "/logs") ||
		isRouteActive(pathname, linkToAuditing) ||
		isRouteActive(pathname, "/connectionlog") ||
		isRouteActive(pathname, "/ai-gateway")
	) {
		return ["logs"];
	}
	return [];
};
