import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import type { WorkspaceApp } from "#/api/typesGenerated";
import {
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
} from "#/testHelpers/entities";
import { withProxyProvider } from "#/testHelpers/storybook";
import type { WorkspacePreviewRightPanelTab } from "../../utils/rightPanelTabs";
import { WorkspaceTabPanel } from "./WorkspaceTabPanel";

const codeServerApp: WorkspaceApp = {
	...MockWorkspaceApp,
	id: "code-server",
	slug: "code-server",
	display_name: "code-server",
	icon: "/icon/code.svg",
	health: "healthy",
};

const claudeApp: WorkspaceApp = {
	...MockWorkspaceApp,
	id: "claude-code",
	slug: "claude-code",
	display_name: "Claude Code",
	icon: "/icon/claude.svg",
	command: "claude",
};

const cursorApp: WorkspaceApp = {
	...MockWorkspaceApp,
	id: "cursor",
	slug: "cursor",
	display_name: "Cursor Desktop",
	icon: "/icon/cursor.svg",
	external: true,
	url: "cursor://coder.coder-remote/open",
};

const agent = {
	...MockWorkspaceAgent,
	apps: [codeServerApp, claudeApp, cursorApp],
};

const previews: WorkspacePreviewRightPanelTab[] = [
	{
		id: "workspace_app-1",
		kind: "workspace_app",
		label: "code-server",
		agentId: agent.id,
		appId: codeServerApp.id,
	},
	{
		id: "port-1",
		kind: "port",
		label: ":3000",
		agentId: agent.id,
		port: 3000,
		protocol: "http",
	},
];

const meta = {
	title: "pages/AgentsPage/WorkspaceTabPanel",
	component: WorkspaceTabPanel,
	args: {
		workspace: MockWorkspace,
		agent,
		host: "*.apps.example.com",
		isRunning: true,
		previews,
		activePreviewId: "port-1",
		isVisible: true,
		onActivePreviewChange: fn(),
		onClosePreview: fn(),
		onOpenWorkspaceApp: fn(),
		onOpenCommandApp: fn(),
		onOpenPort: fn(),
	},
	decorators: [
		withProxyProvider(),
		(Story) => (
			<div style={{ width: 480, height: 500 }}>
				<Story />
			</div>
		),
	],
} satisfies Meta<typeof WorkspaceTabPanel>;

export default meta;
type Story = StoryObj<typeof meta>;

export const EmptyState: Story = {
	args: {
		previews: [],
		activePreviewId: null,
	},
};

export const EmptyStateMenuOpen: Story = {
	args: {
		previews: [],
		activePreviewId: null,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Open an app or port" }),
		);
	},
};

export const WithPreviewChips: Story = {
	// The port preview loads an external iframe, which Pixel cannot capture.
	parameters: { pixel: { exclude: true } },
};

/** Enough previews that the chips no longer fit, so the strip collapses into a selector. */
export const ManyPreviewsSelector: Story = {
	parameters: { pixel: { exclude: true } },
	args: {
		previews: Array.from({ length: 8 }, (_, index) => ({
			id: `port-${3000 + index}`,
			kind: "port" as const,
			label: `:${3000 + index}`,
			agentId: agent.id,
			port: 3000 + index,
			protocol: "http" as const,
		})),
		activePreviewId: "port-3002",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(await canvas.findByRole("button", { name: ":3002" }));
	},
};

export const WorkspaceStopped: Story = {
	args: {
		previews: [],
		activePreviewId: null,
		isRunning: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Open an app or port" }),
		);
	},
};
