import type { Meta, StoryObj } from "@storybook/react-vite";
import { MonitorIcon } from "lucide-react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import type { WorkspaceApp } from "#/api/typesGenerated";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import { withProxyProvider } from "#/testHelpers/storybook";
import { WorkspacePill } from "./WorkspacePill";

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

const defaultProps = {
	name: "my-workspace",
	route: "/@admin/my-workspace",
	statusIcon: <MonitorIcon className="size-3" />,
	statusLabel: "Workspace running",
	chatId: "chat-abc-123",
} satisfies Partial<React.ComponentProps<typeof WorkspacePill>>;

const externalApp: WorkspaceApp = {
	id: "jetbrains-app",
	slug: "jetbrains-gateway",
	display_name: "JetBrains Gateway",
	external: true,
	url: "jetbrains-gateway://connect?$SESSION_TOKEN",
	subdomain: false,
	health: "disabled",
	sharing_level: "owner",
	hidden: false,
	open_in: "slim-window",
	statuses: [],
};

const cursorApp: WorkspaceApp = {
	id: "cursor-app",
	slug: "cursor",
	display_name: "Cursor",
	external: true,
	url: "cursor://coder.coder-remote/open?$SESSION_TOKEN",
	subdomain: false,
	health: "disabled",
	sharing_level: "owner",
	hidden: false,
	open_in: "slim-window",
	statuses: [],
	icon: "/icon/cursor.svg",
};

/** Agent with all built-in display apps and user-configured external apps. */
const agentWithApps = {
	...MockWorkspaceAgent,
	display_apps: ["vscode", "vscode_insiders", "web_terminal"] as const,
	apps: [externalApp, cursorApp],
};

/** Agent with only built-in display apps (no user-configured external apps). */
const agentWithBuiltinsOnly = {
	...MockWorkspaceAgent,
	display_apps: ["vscode", "web_terminal"] as const,
	apps: [],
};

/** Agent with no display apps and no user apps. */
const agentWithNoApps = {
	...MockWorkspaceAgent,
	display_apps: [] as const,
	apps: [],
};

/** Agent with only user-configured external apps (no built-in display apps). */
const agentWithExternalOnly = {
	...MockWorkspaceAgent,
	display_apps: [] as const,
	apps: [externalApp, cursorApp],
};

// ---------------------------------------------------------------------------
// Meta
// ---------------------------------------------------------------------------

const meta: Meta<typeof WorkspacePill> = {
	title: "pages/AgentsPage/WorkspacePill",
	component: WorkspacePill,
	// useAppLink calls useProxy(), so we need the proxy provider for
	// stories that render ExternalAppMenuItem.
	decorators: [withProxyProvider()],
	parameters: {
		layout: "centered",
	},
};
export default meta;
type Story = StoryObj<typeof WorkspacePill>;

// ---------------------------------------------------------------------------
// Stories
// ---------------------------------------------------------------------------

/** Pill with all app types: built-in VS Code, VS Code Insiders, external
 *  apps (JetBrains, Cursor), and terminal. Clicking the pill opens the
 *  dropdown. */
export const WithAllApps: Story = {
	args: {
		...defaultProps,
		workspace: MockWorkspace,
		agent: agentWithApps,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const pill = canvas.getByText("my-workspace");
		await userEvent.click(pill);

		await waitFor(() => {
			const body = within(document.body);
			expect(body.getByText("Open in VS Code")).toBeInTheDocument();
			expect(body.getByText("Open in VS Code Insiders")).toBeInTheDocument();
			expect(body.getByText("JetBrains Gateway")).toBeInTheDocument();
			expect(body.getByText("Cursor")).toBeInTheDocument();
			expect(body.getByText("Open Terminal")).toBeInTheDocument();
			expect(body.getByText("View Workspace")).toBeInTheDocument();
		});
	},
};

/** Pill with only built-in display apps (VS Code and terminal). No
 *  user-configured external apps. */
export const WithBuiltinAppsOnly: Story = {
	args: {
		...defaultProps,
		workspace: MockWorkspace,
		agent: agentWithBuiltinsOnly,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const pill = canvas.getByText("my-workspace");
		await userEvent.click(pill);

		await waitFor(() => {
			const body = within(document.body);
			expect(body.getByText("Open in VS Code")).toBeInTheDocument();
			expect(body.getByText("Open Terminal")).toBeInTheDocument();
			expect(body.getByText("View Workspace")).toBeInTheDocument();
			// No external apps or VS Code Insiders.
			expect(
				body.queryByText("Open in VS Code Insiders"),
			).not.toBeInTheDocument();
		});
	},
};

/** Pill with only user-configured external apps, no built-in display apps. */
export const WithExternalAppsOnly: Story = {
	args: {
		...defaultProps,
		workspace: MockWorkspace,
		agent: agentWithExternalOnly,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const pill = canvas.getByText("my-workspace");
		await userEvent.click(pill);

		await waitFor(() => {
			const body = within(document.body);
			expect(body.getByText("JetBrains Gateway")).toBeInTheDocument();
			expect(body.getByText("Cursor")).toBeInTheDocument();
			expect(body.getByText("View Workspace")).toBeInTheDocument();
			// No built-in apps.
			expect(body.queryByText("Open in VS Code")).not.toBeInTheDocument();
			expect(body.queryByText("Open Terminal")).not.toBeInTheDocument();
		});
	},
};

/** When the agent has no apps at all, the pill renders as a simple link
 *  to the workspace page instead of a dropdown trigger. */
export const NoApps: Story = {
	args: {
		...defaultProps,
		workspace: MockWorkspace,
		agent: agentWithNoApps,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const link = canvas.getByText("my-workspace");
		// The pill should be a link, not a dropdown trigger.
		expect(link.closest("a")).toHaveAttribute(
			"href",
			expect.stringContaining("/@admin/my-workspace"),
		);
	},
};
