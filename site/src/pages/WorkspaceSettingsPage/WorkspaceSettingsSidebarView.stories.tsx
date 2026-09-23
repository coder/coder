import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { useLocation } from "react-router";
import { expect, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { CollapsibleSidebar } from "#/components/Sidebar/CollapsibleSidebar";
import { MockWorkspace } from "#/testHelpers/entities";
import {
	WorkspaceSettingsSidebarHeader,
	WorkspaceSettingsSidebarView,
} from "./WorkspaceSettingsSidebarView";

const STORAGE_KEY = "story-workspace-settings-sidebar";

/** Exposes the router location so play functions can assert on it. */
const LocationProbe: FC = () => {
	const { pathname } = useLocation();
	return (
		<p role="status" aria-label="Current location">
			{pathname}
		</p>
	);
};

const BASE = `/@${MockWorkspace.owner_name}/${MockWorkspace.name}/settings`;

const routing = (path: string) =>
	reactRouterParameters({
		location: { path },
		routing: [
			{ path: BASE, useStoryElement: true },
			...["parameters", "schedule", "sharing"].map((segment) => ({
				path: `${BASE}/${segment}`,
				useStoryElement: true,
			})),
		],
	});

const meta: Meta<typeof WorkspaceSettingsSidebarView> = {
	title: "pages/WorkspaceSettingsPage/WorkspaceSettingsSidebarView",
	component: WorkspaceSettingsSidebarView,
	// Stories share the page, so start expanded every time.
	beforeEach: () => {
		localStorage.setItem(STORAGE_KEY, "expanded");
		return () => localStorage.removeItem(STORAGE_KEY);
	},
	decorators: [
		(Story) => (
			<div className="flex">
				<div className="relative border-0 border-r border-solid border-border">
					<CollapsibleSidebar
						label="Workspace settings"
						storageKey={STORAGE_KEY}
						header={<WorkspaceSettingsSidebarHeader />}
					>
						<Story />
					</CollapsibleSidebar>
				</div>
				<LocationProbe />
			</div>
		),
	],
	parameters: { reactRouter: routing(BASE) },
	args: {
		workspace: MockWorkspace,
		canShareWorkspace: true,
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceSettingsSidebarView>;

/** Both groups expanded, General active. */
export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Workspace settings")).toBeVisible();
		expect(canvas.getByText(MockWorkspace.name)).toBeVisible();
		expect(canvas.getByText(MockWorkspace.owner_name)).toBeVisible();
		expect(
			canvas.getAllByRole("link").map((link) => link.textContent?.trim()),
		).toEqual(["General", "Parameters", "Schedule", "Sharing"]);
		expect(canvas.getByRole("link", { name: "General" })).toHaveAttribute(
			"aria-current",
			"page",
		);
	},
};

/** Without share permission the Access group disappears entirely. */
export const CannotShare: Story = {
	args: { canShareWorkspace: false },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByRole("link", { name: "Sharing" })).toBeNull();
		expect(canvas.queryByText("Access")).toBeNull();
		expect(canvas.getByText("Workspace")).toBeVisible();
	},
};

export const ScheduleActive: Story = {
	parameters: { reactRouter: routing(`${BASE}/schedule`) },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByRole("link", { name: "Schedule" })).toHaveAttribute(
			"aria-current",
			"page",
		);
		expect(canvas.getByRole("link", { name: "General" })).not.toHaveAttribute(
			"aria-current",
		);
	},
};

/** The icon rail shows the template icon, which re-expands the sidebar. */
export const Collapsed: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Collapse sidebar" }),
		);

		const icon = canvas.getByRole("button", { name: MockWorkspace.name });
		expect(icon).toBeVisible();
		expect(canvas.queryByRole("link")).toBeNull();

		await userEvent.click(icon);
		expect(canvas.getByRole("link", { name: "Sharing" })).toBeVisible();
		expect(canvas.getByText(MockWorkspace.owner_name)).toBeVisible();
	},
};

// Group headings are labels, not controls: clicking one neither
// navigates nor hides its links.
export const GroupHeadingIsStatic: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByRole("button", { name: "Workspace" })).toBeNull();
		await userEvent.click(canvas.getByText("Workspace"));
		expect(canvas.getByRole("link", { name: "Parameters" })).toBeVisible();
		expect(
			canvas.getByRole("status", { name: "Current location" }),
		).toHaveTextContent(BASE);
	},
};
