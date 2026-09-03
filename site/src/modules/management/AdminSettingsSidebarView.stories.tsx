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
import { AdminSettingsSidebarView } from "./AdminSettingsSidebarView";

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
	// Each story gets its own persisted accordion state, seeded from the
	// `openSections` parameter (or cleared), so interactions in one story
	// cannot leak into another.
	decorators: [
		(Story, { args, parameters }) => {
			const key = args.openSectionsStorageKey ?? "admin-sidebar-open-sections";
			const seed = parameters.openSections as string[] | undefined;
			if (seed) {
				localStorage.setItem(key, JSON.stringify(seed));
			} else {
				localStorage.removeItem(key);
			}
			return (
				<div className="w-60">
					<Story />
					<LocationProbe />
				</div>
			);
		},
	],
	parameters: { reactRouter: routing("/deployment/overview") },
	args: {
		permissions: MockPermissions,
		showOrganizations: true,
		hasPremiumLicense: false,
		experiments: [],
		buildInfo: MockBuildInfo,
		canViewOrganizations: true,
		organizations: [MockOrganization, MockOrganization2],
		activeOrganization: MockOrganization,
		orgPermissions: MockOrganizationPermissions,
		canViewAuditLog: true,
		canViewConnectionLog: true,
		canViewAIBridge: true,
	},
};

export default meta;
type Story = StoryObj<typeof AdminSettingsSidebarView>;

/** First load with no persisted state: Deployment > General open, Overview active. */
export const Default: Story = {
	args: { openSectionsStorageKey: "story-admin-default" },
};

export const OrganizationsOpen: Story = {
	args: { openSectionsStorageKey: "story-admin-organizations" },
	parameters: {
		openSections: ["organizations", "organizations-provisioners"],
		reactRouter: routing("/organizations/my-organization/provisioners"),
	},
};

export const AIOpen: Story = {
	args: { openSectionsStorageKey: "story-admin-ai" },
	parameters: {
		openSections: ["ai", "ai-coder-agents"],
		reactRouter: routing("/ai/settings/mcp-servers"),
	},
};

export const LogsOpen: Story = {
	args: { openSectionsStorageKey: "story-admin-logs" },
	parameters: {
		openSections: ["logs"],
		reactRouter: routing("/audit"),
	},
};

export const AllSectionsOpen: Story = {
	args: { openSectionsStorageKey: "story-admin-all" },
	parameters: {
		openSections: [
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
	args: { openSectionsStorageKey: "story-admin-collapsed" },
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
		openSectionsStorageKey: "story-admin-no-ai",
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
		openSectionsStorageKey: "story-admin-no-logs",
		canViewAuditLog: false,
		canViewConnectionLog: false,
		canViewAIBridge: false,
	},
};

export const NoOrganizations: Story = {
	args: {
		openSectionsStorageKey: "story-admin-no-orgs",
		canViewOrganizations: false,
	},
};

/** Can see the organizations section but has no permissions in the org. */
export const OrganizationViewerOnly: Story = {
	args: {
		openSectionsStorageKey: "story-admin-org-viewer",
		orgPermissions: MockNoOrganizationPermissions,
	},
	parameters: {
		openSections: ["organizations"],
		reactRouter: routing("/organizations/my-organization"),
	},
};

export const MemberWithAuditOnly: Story = {
	args: {
		openSectionsStorageKey: "story-admin-member",
		permissions: MockNoPermissions,
		canViewOrganizations: false,
		canViewConnectionLog: false,
		canViewAIBridge: false,
	},
	parameters: {
		openSections: ["logs"],
		reactRouter: routing("/audit"),
	},
};

export const ExpandInfrastructure: Story = {
	args: { openSectionsStorageKey: "story-admin-expand-infra" },
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
	args: { openSectionsStorageKey: "story-admin-header-click" },
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
	args: { openSectionsStorageKey: "story-admin-narrow" },
	decorators: [
		(Story) => {
			localStorage.setItem("story-admin-narrow-width", "expanded");
			return (
				<CollapsibleSidebar storageKey="story-admin-narrow-width">
					<Story />
				</CollapsibleSidebar>
			);
		},
	],
	parameters: {
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
	args: { openSectionsStorageKey: "story-admin-wide-peek" },
	decorators: [
		(Story) => {
			localStorage.setItem("story-admin-wide-peek-width", "expanded");
			return (
				<CollapsibleSidebar
					storageKey="story-admin-wide-peek-width"
					preferCollapsed
				>
					<Story />
				</CollapsibleSidebar>
			);
		},
	],
	parameters: {
		openSections: ["logs"],
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
	args: { openSectionsStorageKey: "story-admin-wide-collapsed" },
	decorators: [
		(Story) => {
			localStorage.setItem("story-admin-wide-collapsed-width", "collapsed");
			return (
				<CollapsibleSidebar
					storageKey="story-admin-wide-collapsed-width"
					preferCollapsed
				>
					<Story />
				</CollapsibleSidebar>
			);
		},
	],
	parameters: {
		openSections: ["logs"],
		reactRouter: routing("/audit"),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// No peek: only the icon rail renders from the start.
		expect(canvas.queryByRole("button", { name: "Logs" })).toBeNull();
		expect(canvas.queryByRole("link", { name: "Audit logs" })).toBeNull();
	},
};
