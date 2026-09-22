import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { useLocation } from "react-router";
import { expect, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { CollapsibleSidebar } from "#/components/Sidebar/CollapsibleSidebar";
import { SidebarContext } from "#/components/Sidebar/SidebarContext";
import { MockWorkspace } from "#/testHelpers/entities";
import {
	WorkspaceSettingsSidebarHeader,
	WorkspaceSettingsSidebarView,
} from "./WorkspaceSettingsSidebarView";

/** Exposes the router location so play functions can assert on it. */
const LocationProbe: FC = () => {
	const { pathname } = useLocation();
	return <div data-testid="location">{pathname}</div>;
};

const BASE = `/@${MockWorkspace.owner_name}/${MockWorkspace.name}/settings`;
const ROUTES = [
	BASE,
	`${BASE}/parameters`,
	`${BASE}/schedule`,
	`${BASE}/sharing`,
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

const meta: Meta<typeof WorkspaceSettingsSidebarView> = {
	title: "pages/WorkspaceSettingsPage/WorkspaceSettingsSidebarView",
	component: WorkspaceSettingsSidebarView,
	decorators: [
		(Story, { parameters }) => {
			// Stories that mount a real CollapsibleSidebar supply the header
			// through its header slot instead.
			const usesRealSidebar = Boolean(parameters.realSidebar);
			return (
				<div className="w-60">
					{!usesRealSidebar && <WorkspaceSettingsSidebarHeader />}
					<Story />
					<LocationProbe />
				</div>
			);
		},
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

/** The icon rail: the template icon re-expands the sidebar. */
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: MockWorkspace.name }),
		).toBeVisible();
		expect(canvas.queryByRole("link")).toBeNull();
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
		expect(canvas.getByTestId("location")).toHaveTextContent(BASE);
	},
};

// Inside a real CollapsibleSidebar: the header is pinned at 56px and the
// list geometry matches the user settings sidebar.
export const LayoutMetrics: Story = {
	decorators: [
		(Story) => {
			localStorage.setItem("story-workspace-metrics-width", "expanded");
			return (
				<CollapsibleSidebar
					storageKey="story-workspace-metrics-width"
					header={<WorkspaceSettingsSidebarHeader />}
				>
					<Story />
				</CollapsibleSidebar>
			);
		},
	],
	parameters: { realSidebar: true },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const sidebar = canvasElement.querySelector("[data-sidebar-container]");
		if (!(sidebar instanceof HTMLElement)) {
			throw new Error("sidebar container not rendered");
		}
		const edge = sidebar.getBoundingClientRect();
		const rect = (element: Element) => element.getBoundingClientRect();

		const header = canvas.getByTestId("sidebar-panel").firstElementChild;
		const heading = canvas.getByText("Workspace").parentElement;
		const general = canvas.getByRole("link", { name: "General" });
		const parameters = canvas.getByRole("link", { name: "Parameters" });
		const line = general.parentElement;
		if (!header || !heading || !line) {
			throw new Error("sidebar rows not rendered");
		}

		expect(edge.width).toBe(240);
		expect(rect(header).height).toBe(56);
		expect(rect(heading).height).toBe(32);
		expect(rect(canvas.getByText("Workspace")).left - edge.left).toBe(16);
		expect(rect(general).height).toBe(32);
		expect(rect(parameters).top - rect(general).bottom).toBe(0);
		expect(rect(line).top - rect(heading).bottom).toBe(0);
		expect(rect(general).left + 8 - (rect(line).left + line.clientLeft)).toBe(
			12,
		);
	},
};
