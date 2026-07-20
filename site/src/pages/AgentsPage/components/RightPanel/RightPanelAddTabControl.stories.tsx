import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type { WorkspaceApp } from "#/api/typesGenerated";
import {
	MockListeningPortsResponse,
	MockSharedPortsResponse,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
} from "#/testHelpers/entities";
import { withProxyProvider } from "#/testHelpers/storybook";
import { RightPanelAddTabControl } from "./RightPanelAddTabControl";

const embeddableApp: WorkspaceApp = {
	...MockWorkspaceApp,
	id: "preview-app",
	slug: "preview",
	display_name: "Preview",
	health: "healthy",
	external: false,
	hidden: false,
	command: undefined,
};

const commandApp: WorkspaceApp = {
	...MockWorkspaceApp,
	id: "command-app",
	slug: "claude-code",
	display_name: "Claude Code",
	health: "healthy",
	external: false,
	hidden: false,
	command: "claude",
};

const externalApp: WorkspaceApp = {
	...MockWorkspaceApp,
	id: "docs-app",
	slug: "docs",
	display_name: "Documentation",
	health: "healthy",
	external: true,
	url: "https://example.com",
	open_in: "tab",
};

const meta = {
	title: "pages/AgentsPage/components/RightPanel/RightPanelAddTabControl",
	component: RightPanelAddTabControl,
	args: {
		workspace: MockWorkspace,
		agent: {
			...MockWorkspaceAgent,
			apps: [embeddableApp, commandApp, externalApp],
		},
		host: "*.apps.example.com",
		isRunning: true,
		onNewTerminal: fn(),
		onOpenWorkspaceApp: fn(),
		onOpenCommandApp: fn(),
		onOpenPort: fn(),
	},
	parameters: {
		layout: "centered",
		queries: [
			{
				key: ["portForward", MockWorkspaceAgent.id],
				data: MockListeningPortsResponse,
			},
			{
				key: ["sharedPorts", MockWorkspace.id],
				data: MockSharedPortsResponse,
			},
		],
	},
	decorators: [
		withProxyProvider(),
		(Story) => (
			<div style={{ padding: 16 }}>
				<Story />
			</div>
		),
	],
} satisfies Meta<typeof RightPanelAddTabControl>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const openMenu = async () => {
			await userEvent.click(canvas.getByLabelText("Add panel"));
		};

		await openMenu();
		await waitFor(() => {
			expect(body.getByText("New Terminal")).toBeInTheDocument();
			expect(body.getByText("Preview")).toBeInTheDocument();
			expect(body.getByText("Claude Code")).toBeInTheDocument();
			expect(body.getByText("Documentation")).toBeInTheDocument();
			expect(body.getByText("Ports (3)")).toBeInTheDocument();
		});

		// Radix closes the menu after each item click, so reopen between
		// callback assertions.
		await userEvent.click(body.getByText("New Terminal"));
		await expect(args.onNewTerminal).toHaveBeenCalledTimes(1);

		await openMenu();
		await userEvent.click(body.getByText("Preview"));
		await expect(args.onOpenWorkspaceApp).toHaveBeenCalledWith(
			expect.objectContaining({ slug: "preview" }),
		);

		await openMenu();
		await userEvent.click(body.getByText("Claude Code"));
		await expect(args.onOpenCommandApp).toHaveBeenCalledWith(
			expect.objectContaining({ slug: "claude-code" }),
		);

		await openMenu();
		await userEvent.hover(body.getByText("Ports (3)"));
		await waitFor(() => {
			expect(body.getByText("Listening Ports")).toBeInTheDocument();
			expect(body.getByText("8080")).toBeInTheDocument();
		});
		await userEvent.click(body.getByText("8080"));
		await expect(args.onOpenPort).toHaveBeenCalledWith(
			expect.objectContaining({ port: 8080 }),
		);
	},
};

export const DisconnectedWorkspace: Story = {
	args: {
		agent: {
			...MockWorkspaceAgent,
			status: "disconnected",
			apps: [embeddableApp, commandApp],
		},
		isRunning: false,
	},
	parameters: {
		// No seeded port data: the ports query does not run while the agent
		// is disconnected, so the trigger shows no count.
		queries: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByLabelText("Add panel"));

		const body = within(document.body);
		await waitFor(() => {
			// Creating a terminal is disabled while the workspace is not
			// running, since the agent cannot accept a PTY session.
			const terminalItem = body
				.getByText("New Terminal")
				.closest("[role=menuitem]");
			expect(terminalItem).toHaveAttribute("aria-disabled", "true");

			// App items render but are disabled while the workspace is not
			// running.
			const previewItem = body.getByText("Preview").closest("[role=menuitem]");
			expect(previewItem).toHaveAttribute("aria-disabled", "true");
			const commandItem = body
				.getByText("Claude Code")
				.closest("[role=menuitem]");
			expect(commandItem).toHaveAttribute("aria-disabled", "true");

			// Ports sub-trigger is disabled, with no stale port count.
			const portsItem = body.getByText("Ports").closest("[role=menuitem]");
			expect(portsItem).toHaveAttribute("aria-disabled", "true");
		});
	},
};
