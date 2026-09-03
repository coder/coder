import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { useLocation } from "react-router";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { CollapsibleSidebar } from "#/components/Sidebar/CollapsibleSidebar";
import { SidebarContext } from "#/components/Sidebar/SidebarContext";
import {
	MockBuildInfo,
	MockNoOrganizationPermissions,
	MockNoPermissions,
	MockOrganization,
	MockOrganization2,
	MockOrganizationPermissions,
	MockPermissions,
} from "#/testHelpers/entities";
import {
	AdminSettingsSidebarHeader,
	AdminSettingsSidebarView,
} from "./AdminSettingsSidebarView";

/** Exposes the router location so play functions can assert on it. */
const LocationProbe: FC = () => {
	const { pathname } = useLocation();
	return <div data-testid="location">{pathname}</div>;
};

const ROUTES = [
	"/deployment/overview",
	"/deployment/security",
	"/deployment/users",
	"/organizations/my-organization",
	"/organizations/my-organization/provisioners",
	"/ai/settings/models",
	"/ai/settings/mcp-servers",
	"/audit",
	"/connectionlog",
	"/health",
];

const routing = (path: string) =>
	reactRouterParameters({
		location: { path },
		routing: [
			{ path: ROUTES[0], useStoryElement: true },
			...ROUTES.slice(1).map((route) => ({
				path: route,
				useStoryElement: true,
			})),
		],
	});

const meta: Meta<typeof AdminSettingsSidebarView> = {
	title: "modules/management/AdminSettingsSidebarView",
	component: AdminSettingsSidebarView,
	decorators: [
		(Story, { parameters }) => {
			// Stories that mount a real CollapsibleSidebar supply the header
			// through its header slot instead.
			const usesRealSidebar = Boolean(parameters.realSidebar);
			return (
				<div className="w-60">
					{!usesRealSidebar && <AdminSettingsSidebarHeader />}
					<Story />
					<LocationProbe />
				</div>
			);
		},
	],
	parameters: { reactRouter: routing("/deployment/overview") },
	args: {
		permissions: MockPermissions,
		hidePremiumTab: false,
		experiments: [],
		buildInfo: MockBuildInfo,
		canViewOrganizations: true,
		organizations: [MockOrganization, MockOrganization2],
		activeOrganization: MockOrganization,
		orgPermissions: MockOrganizationPermissions,
		canViewAuditLog: true,
		canViewConnectionLog: true,
		canViewAIBridge: true,
		canAccessOrganizationModels: false,
		canShareOrganizationMCPServers: false,
	},
};

export default meta;
type Story = StoryObj<typeof AdminSettingsSidebarView>;

/** First load with no persisted state: Deployment > General open, Overview active. */
export const Default: Story = {};

export const OrganizationsOpen: Story = {
	parameters: {
		reactRouter: routing("/organizations/my-organization/provisioners"),
	},
};

export const AIOpen: Story = {
	parameters: {
		reactRouter: routing("/ai/settings/mcp-servers"),
	},
};

export const LogsOpen: Story = {
	parameters: {
		reactRouter: routing("/audit"),
	},
};

export const AllSectionsOpen: Story = {
	args: {
		initialOpenSections: [
			"deployment",
			"deployment-general",
			"deployment-infrastructure",
			"deployment-authentication",
			"organizations",
			"organizations-provisioners",
			"ai",
			"ai-coder-agents",
			"logs",
		],
	},
};

export const Collapsed: Story = {
	decorators: [
		(Story) => (
			<SidebarContext.Provider
				value={{ collapsed: true, expand: () => {}, toggle: () => {} }}
			>
				<Story />
			</SidebarContext.Provider>
		),
	],
};

export const NoAI: Story = {
	args: {
		permissions: {
			...MockPermissions,
			viewAnyAIProvider: false,
			viewAIGatewayKeys: false,
			editDeploymentConfig: false,
		},
	},
};

export const NoLogs: Story = {
	args: {
		canViewAuditLog: false,
		canViewConnectionLog: false,
		canViewAIBridge: false,
	},
};

export const NoOrganizations: Story = {
	args: {
		canViewOrganizations: false,
	},
};

/** Can see the organizations section but has no permissions in the org. */
export const OrganizationViewerOnly: Story = {
	args: {
		orgPermissions: MockNoOrganizationPermissions,
	},
	parameters: {
		reactRouter: routing("/organizations/my-organization"),
	},
};

export const MemberWithAuditOnly: Story = {
	args: {
		permissions: MockNoPermissions,
		canViewOrganizations: false,
		canViewConnectionLog: false,
		canViewAIBridge: false,
	},
	parameters: {
		reactRouter: routing("/audit"),
	},
};

export const ExpandInfrastructure: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByRole("link", { name: "Security" })).toBeNull();
		await userEvent.click(
			canvas.getByRole("button", { name: "Infrastructure" }),
		);
		await waitFor(() =>
			expect(canvas.getByRole("link", { name: "Security" })).toBeVisible(),
		);
		// Expanding a section never navigates.
		expect(canvas.getByTestId("location")).toHaveTextContent(
			"/deployment/overview",
		);
		// General stays open; nothing auto-closes.
		expect(canvas.getByRole("link", { name: "Overview" })).toBeVisible();
	},
};

export const HeaderClickDoesNotNavigate: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Deployment" }));
		await waitFor(() =>
			expect(canvas.queryByRole("link", { name: "Overview" })).toBeNull(),
		);
		expect(canvas.getByTestId("location")).toHaveTextContent(
			"/deployment/overview",
		);
		await userEvent.click(canvas.getByRole("button", { name: "Logs" }));
		await waitFor(() =>
			expect(canvas.getByRole("link", { name: "Audit logs" })).toBeVisible(),
		);
		expect(canvas.getByTestId("location")).toHaveTextContent(
			"/deployment/overview",
		);
	},
};

// Renders inside a real CollapsibleSidebar at a narrow viewport. Even
// though the persisted width preference is "expanded", the sidebar must
// auto-collapse to the icon rail below the lg breakpoint.
export const NarrowViewportAutoCollapse: Story = {
	decorators: [
		(Story) => {
			localStorage.setItem("story-admin-narrow-width", "expanded");
			return (
				<CollapsibleSidebar
					storageKey="story-admin-narrow-width"
					header={<AdminSettingsSidebarHeader />}
				>
					<Story />
				</CollapsibleSidebar>
			);
		},
	],
	parameters: {
		realSidebar: true,
		viewport: { defaultViewport: "iphone12" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Collapsed accordions render icon-only triggers, so the labeled
		// "Deployment" trigger must not exist at a narrow viewport.
		await waitFor(() => {
			expect(canvas.queryByRole("button", { name: "Deployment" })).toBeNull();
		});
	},
};

// Wide-content pages: the sidebar settles collapsed. If the preference
// was expanded, the nav peeks as a flyout first, then collapses.
export const WideContentPeek: Story = {
	decorators: [
		(Story) => {
			localStorage.setItem("story-admin-wide-peek-width", "expanded");
			return (
				<CollapsibleSidebar
					storageKey="story-admin-wide-peek-width"
					header={<AdminSettingsSidebarHeader />}
					preferCollapsed
				>
					<Story />
				</CollapsibleSidebar>
			);
		},
	],
	parameters: {
		realSidebar: true,
		reactRouter: routing("/audit"),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// The peek shows the expanded nav with labeled headers.
		await canvas.findByRole("button", { name: "Logs" });
		// After the peek the sidebar settles to the icon rail.
		await waitFor(
			() => {
				expect(canvas.queryByRole("button", { name: "Logs" })).toBeNull();
			},
			{ timeout: 3000 },
		);
	},
};

export const WideContentStartsCollapsedWhenPreferenceCollapsed: Story = {
	decorators: [
		(Story) => {
			localStorage.setItem("story-admin-wide-collapsed-width", "collapsed");
			return (
				<CollapsibleSidebar
					storageKey="story-admin-wide-collapsed-width"
					header={<AdminSettingsSidebarHeader />}
					preferCollapsed
				>
					<Story />
				</CollapsibleSidebar>
			);
		},
	],
	parameters: {
		realSidebar: true,
		reactRouter: routing("/audit"),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// No peek: only the icon rail renders from the start.
		expect(canvas.queryByRole("button", { name: "Logs" })).toBeNull();
		expect(canvas.queryByRole("link", { name: "Audit logs" })).toBeNull();
	},
};

const ALL_SECTIONS = [
	"deployment",
	"deployment-general",
	"deployment-infrastructure",
	"deployment-authentication",
	"organizations",
	"organizations-provisioners",
	"ai",
	"ai-coder-agents",
	"logs",
];

// A short, wide viewport with every section open: the list overflows,
// the header stays pinned, and only the nav list scrolls.
export const TallListScrolls: Story = {
	decorators: [
		(Story) => {
			localStorage.setItem("story-admin-tall-width", "expanded");
			return (
				<CollapsibleSidebar
					storageKey="story-admin-tall-width"
					header={<AdminSettingsSidebarHeader />}
				>
					<Story />
				</CollapsibleSidebar>
			);
		},
	],
	args: { initialOpenSections: ALL_SECTIONS },
	parameters: {
		realSidebar: true,
		viewport: {
			options: {
				shortDesktop: {
					name: "Short desktop",
					styles: { width: "1200px", height: "600px" },
				},
			},
			defaultViewport: "shortDesktop",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const scrollArea = canvas.getByTestId("sidebar-scroll-area");
		const header = canvas.getByRole("button", { name: /admin settings/i });
		const headerTop = header.getBoundingClientRect().top;

		await waitFor(() => {
			expect(scrollArea.scrollHeight).toBeGreaterThan(scrollArea.clientHeight);
		});
		// The page itself does not need to scroll to reveal the list.
		expect(document.documentElement.scrollHeight).toBeLessThanOrEqual(
			window.innerHeight,
		);

		scrollArea.scrollTop = scrollArea.scrollHeight;
		await waitFor(() => expect(scrollArea.scrollTop).toBeGreaterThan(0));
		expect(header.getBoundingClientRect().top).toBe(headerTop);
		expect(header).toBeVisible();
	},
};

// Measures the rendered geometry against the design spec (240px sidebar,
// logical px): 40px section rows with the icon 16px from the edge and the
// chevron 12px from the edge, 40px nested headers aligned with the parent
// label, 32px nested leaves 20px right of the connecting line, 4px between
// leaves, 16px before the next nested header, 12px between sections.
export const LayoutMetrics: Story = {
	decorators: [
		(Story) => {
			localStorage.setItem("story-admin-metrics-width", "expanded");
			return (
				<CollapsibleSidebar
					storageKey="story-admin-metrics-width"
					header={<AdminSettingsSidebarHeader />}
				>
					<Story />
				</CollapsibleSidebar>
			);
		},
	],
	args: {
		initialOpenSections: ["deployment", "deployment-general", "organizations"],
	},
	parameters: {
		realSidebar: true,
		reactRouter: routing("/organizations/my-organization"),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const sidebar = canvasElement.querySelector("[data-sidebar-container]");
		if (!(sidebar instanceof HTMLElement)) {
			throw new Error("sidebar container not rendered");
		}
		const edge = sidebar.getBoundingClientRect();
		const rect = (element: Element) => element.getBoundingClientRect();

		const headerToggle = canvas.getByRole("button", {
			name: /admin settings/i,
		});
		const deployment = canvas.getByRole("button", { name: "Deployment" });
		const deploymentIcon = deployment.querySelector("svg");
		const deploymentChevron = deployment.querySelectorAll("svg")[1];
		const deploymentLabel = canvas.getByText("Deployment");
		const general = canvas.getByRole("button", { name: "General" });
		const generalChevron = general.querySelector("svg");
		const generalLabel = canvas.getByText("General");
		const overview = canvas.getByRole("link", { name: "Overview" });
		const licenses = canvas.getByRole("link", { name: "Licenses" });
		const infrastructure = canvas.getByRole("button", {
			name: "Infrastructure",
		});
		const organization = canvas.getByRole("button", { name: "Organizations" });
		const members = canvas.getByRole("link", { name: "Members" });
		const healthcheck = canvas.getByRole("link", { name: "Healthcheck" });
		const healthcheckIcon = healthcheck.querySelector("svg");
		if (
			!deploymentIcon ||
			!deploymentChevron ||
			!generalChevron ||
			!healthcheckIcon
		) {
			throw new Error("section icons not rendered");
		}
		const lastGeneralLeaf = overview.parentElement?.lastElementChild;
		const line = overview.parentElement;
		if (!lastGeneralLeaf || !line) {
			throw new Error("nested list not rendered");
		}

		const metrics = {
			sidebarWidth: edge.width,
			headerRowHeight: rect(headerToggle).height,
			headerTopPadding: rect(headerToggle).top - edge.top,
			sectionRowHeight: rect(deployment).height,
			iconLeft: rect(deploymentIcon).left - edge.left,
			iconSize: rect(deploymentIcon).width,
			labelGap: rect(deploymentLabel).left - rect(deploymentIcon).right,
			chevronRight: edge.right - rect(deploymentChevron).right,
			nestedHeaderHeight: rect(general).height,
			nestedLabelOffset: rect(generalLabel).left - rect(deploymentLabel).left,
			nestedChevronOffset:
				rect(generalChevron).right - rect(deploymentChevron).right,
			nestedLeafHeight: rect(overview).height,
			// Measured from the inner edge of the 1px connecting line to the
			// text start (the leaf has 8px of horizontal padding).
			leafTextFromLine:
				rect(overview).left + 8 - (rect(line).left + line.clientLeft),
			leafGap: rect(licenses).top - rect(overview).bottom,
			listToNextHeader: rect(infrastructure).top - rect(lastGeneralLeaf).bottom,
			sectionLeafHeight: rect(members).height,
			sectionGap:
				rect(organization).top - rect(deployment.parentElement!).bottom,
			healthcheckRowHeight: rect(healthcheck).height,
			healthcheckIconLeft: rect(healthcheckIcon).left - edge.left,
		};
		expect(metrics.sidebarWidth).toBe(240);
		expect(metrics.headerRowHeight).toBe(40);
		expect(metrics.headerTopPadding).toBe(12);
		expect(metrics.sectionRowHeight).toBe(40);
		expect(metrics.iconLeft).toBe(16);
		expect(metrics.iconSize).toBe(16);
		expect(metrics.labelGap).toBe(8);
		expect(metrics.chevronRight).toBe(12);
		expect(metrics.nestedHeaderHeight).toBe(40);
		expect(metrics.nestedLabelOffset).toBe(0);
		expect(metrics.nestedChevronOffset).toBe(0);
		expect(metrics.nestedLeafHeight).toBe(32);
		expect(metrics.leafTextFromLine).toBe(20);
		expect(metrics.leafGap).toBe(4);
		expect(metrics.listToNextHeader).toBe(16);
		expect(metrics.sectionLeafHeight).toBe(40);
		expect(metrics.sectionGap).toBe(12);
		expect(metrics.healthcheckRowHeight).toBe(40);
		expect(metrics.healthcheckIconLeft).toBe(16);
	},
};

// A refresh (fresh mount) opens only the chain for the current route.
export const RefreshCollapsesOtherSections: Story = {
	parameters: { reactRouter: routing("/ai/settings/mcp-servers") },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() =>
			expect(canvas.getByRole("link", { name: "MCP servers" })).toBeVisible(),
		);
		expect(canvas.getByRole("link", { name: "Providers" })).toBeVisible();
		// Other sections start collapsed: their headers render, children do not.
		expect(canvas.getByRole("button", { name: "Deployment" })).toBeVisible();
		expect(canvas.queryByRole("button", { name: "General" })).toBeNull();
		expect(canvas.queryByRole("link", { name: "Members" })).toBeNull();
		expect(canvas.queryByRole("link", { name: "Audit logs" })).toBeNull();
	},
};
