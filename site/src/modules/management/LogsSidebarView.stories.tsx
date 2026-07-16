import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { CollapsibleSidebar } from "#/components/Sidebar/CollapsibleSidebar";
import { SidebarContext } from "#/components/Sidebar/SidebarContext";
import LogsSidebarView from "./LogsSidebarView";

const meta: Meta<typeof LogsSidebarView> = {
	title: "modules/management/LogsSidebarView",
	component: LogsSidebarView,
	args: {
		canViewAuditLog: true,
		canViewConnectionLog: true,
		canViewAIBridge: true,
		activeSection: "audit",
	},
};

export default meta;
type Story = StoryObj<typeof LogsSidebarView>;

export const AuditActive: Story = {};

export const ConnectionActive: Story = {
	args: {
		activeSection: "connection",
	},
};

export const AISessionsActive: Story = {
	args: {
		activeSection: "ai-sessions",
	},
};

export const Collapsed: Story = {
	decorators: [
		(Story) => (
			<SidebarContext.Provider
				value={{
					collapsed: true,
					expand: () => {},
					collapse: () => {},
					toggle: () => {},
				}}
			>
				<Story />
			</SidebarContext.Provider>
		),
	],
};

export const NoAuditLog: Story = {
	args: {
		canViewAuditLog: false,
	},
};

export const NoConnectionLog: Story = {
	args: {
		canViewConnectionLog: false,
	},
};

export const NoAIBridge: Story = {
	args: {
		canViewAIBridge: false,
	},
};

export const AuditOnly: Story = {
	args: {
		canViewConnectionLog: false,
		canViewAIBridge: false,
	},
};

// Overlay stories render the view inside a real CollapsibleSidebar in
// overlay mode, next to a fake content column, so the flyout behavior
// can be exercised. Each story uses its own storage key, seeded by a
// decorator, so interactions cannot leak between stories.
const withOverlaySidebar =
	(storageKey: string, persisted?: "collapsed" | "expanded", peek?: boolean) =>
	(Story: React.ComponentType) => {
		if (persisted === undefined) {
			localStorage.removeItem(storageKey);
		} else {
			localStorage.setItem(storageKey, persisted);
		}
		return (
			<div className="flex flex-row h-[400px]">
				<div className="border-0 border-r border-solid border-border">
					<CollapsibleSidebar
						storageKey={storageKey}
						overlay
						peekOnMount={peek}
					>
						<Story />
					</CollapsibleSidebar>
				</div>
				<div className="flex-1 min-w-0 p-6 text-sm text-content-secondary">
					Wide table content that the expanded sidebar overlays instead of
					pushing aside.
				</div>
			</div>
		);
	};

const overlayRouting = reactRouterParameters({
	location: { path: "/audit" },
	routing: [
		{ path: "/audit", useStoryElement: true },
		{ path: "/connectionlog", useStoryElement: true },
		{ path: "/ai-gateway/sessions", useStoryElement: true },
	],
});

export const OverlayExpanded: Story = {
	decorators: [withOverlaySidebar("story-logs-overlay-expanded", "expanded")],
	parameters: { reactRouter: overlayRouting },
};

export const OverlayCollapsed: Story = {
	decorators: [withOverlaySidebar("story-logs-overlay-collapsed", "collapsed")],
	parameters: { reactRouter: overlayRouting },
};

export const OverlayNavItemClickCollapses: Story = {
	decorators: [withOverlaySidebar("story-logs-overlay-click", "expanded")],
	parameters: { reactRouter: overlayRouting },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const link = await canvas.findByRole("link", { name: "Connection logs" });
		await userEvent.click(link);
		// Collapsed mode replaces labeled links with icon-only links, so
		// the accessible name disappears once the sidebar collapses.
		await waitFor(() => {
			expect(
				canvas.queryByRole("link", { name: "Connection logs" }),
			).not.toBeInTheDocument();
		});
	},
};

export const OverlayPeekOnMount: Story = {
	decorators: [withOverlaySidebar("story-logs-overlay-peek", undefined, true)],
	parameters: { reactRouter: overlayRouting },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// The peek starts expanded, showing labeled links.
		await canvas.findByRole("link", { name: "Audit logs" });
		// After the peek delay the sidebar auto-collapses to icons.
		await waitFor(
			() => {
				expect(
					canvas.queryByRole("link", { name: "Audit logs" }),
				).not.toBeInTheDocument();
			},
			{ timeout: 3000 },
		);
	},
};
