import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type { WorkspaceApp } from "#/api/typesGenerated";
import {
	MockListeningPortsResponse,
	MockSharedPortsResponse,
	MockStoppedWorkspace,
	MockWorkspace,
	MockWorkspaceAgent,
} from "#/testHelpers/entities";
import { withProxyProvider } from "#/testHelpers/storybook";
import { WorkspacePill } from "./WorkspacePill";

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

const defaultProps = {
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

const hiddenApp: WorkspaceApp = {
	id: "hidden-app",
	slug: "hidden-internal",
	display_name: "Hidden Internal Tool",
	external: false,
	url: "",
	subdomain: false,
	health: "disabled",
	sharing_level: "owner",
	hidden: true,
	open_in: "slim-window",
	statuses: [],
};

const agentWithApps = {
	...MockWorkspaceAgent,
	display_apps: [
		"vscode",
		"vscode_insiders",
		"web_terminal",
		"port_forwarding_helper",
	] as const,
	apps: [externalApp, cursorApp],
};

const agentWithBuiltinsOnly = {
	...MockWorkspaceAgent,
	display_apps: ["vscode", "web_terminal"] as const,
	apps: [],
};

const agentWithNoApps = {
	...MockWorkspaceAgent,
	display_apps: [] as const,
	apps: [],
};

const agentWithExternalOnly = {
	...MockWorkspaceAgent,
	display_apps: [] as const,
	apps: [externalApp, cursorApp],
};

const agentWithHiddenApp = {
	...MockWorkspaceAgent,
	display_apps: ["vscode"] as const,
	apps: [externalApp, hiddenApp],
};

// ---------------------------------------------------------------------------
// Meta
// ---------------------------------------------------------------------------

const meta: Meta<typeof WorkspacePill> = {
	title: "pages/AgentsPage/WorkspacePill",
	component: WorkspacePill,
	// useAppLink and useProxy are called inside sub-components, so we need the
	// proxy provider for all stories. A non-empty wildcard hostname is required
	// so the Ports sub-trigger renders (it is hidden when port-forwarding is not
	// configured).
	decorators: [
		withProxyProvider({
			proxy: {
				proxy: undefined,
				preferredPathAppURL: "",
				preferredWildcardHostname: "*.coder.com",
			},
		}),
	],

	parameters: {
		layout: "centered",
		queries: [{ key: ["me", "apiKey"], data: { key: "mock-api-key" } }],
	},
};
export default meta;
type Story = StoryObj<typeof WorkspacePill>;

// ---------------------------------------------------------------------------
// Stories
// ---------------------------------------------------------------------------

export const WithAllApps: Story = {
	args: {
		...defaultProps,
		workspace: MockWorkspace,
		agent: agentWithApps,
		sshCommand: "ssh coder.test-workspace",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const pill = canvas.getByText("test-workspace");
		await userEvent.click(pill);

		await waitFor(() => {
			const body = within(document.body);
			expect(body.getByText("VS Code")).toBeInTheDocument();
			expect(body.getByText("VS Code Insiders")).toBeInTheDocument();
			expect(body.getByText("JetBrains Gateway")).toBeInTheDocument();
			expect(body.getByText("Cursor")).toBeInTheDocument();
			expect(body.getByText("Terminal")).toBeInTheDocument();
			expect(body.getByText("Copy SSH Command")).toBeInTheDocument();
			expect(body.getByText("View Workspace")).toBeInTheDocument();

			// Verify items are enabled on a running workspace.
			const vscodeItem = body.getByText("VS Code").closest("[role=menuitem]");
			expect(vscodeItem).not.toHaveAttribute("aria-disabled", "true");

			// External apps should be enabled with API key mock.
			const jetbrainsItem = body
				.getByText("JetBrains Gateway")
				.closest("[role=menuitem]");
			expect(jetbrainsItem).not.toHaveAttribute("aria-disabled", "true");

			const cursorItem = body.getByText("Cursor").closest("[role=menuitem]");
			expect(cursorItem).not.toHaveAttribute("aria-disabled", "true");
		});
	},
};

export const WithRemoveAction: Story = {
	args: {
		...defaultProps,
		workspace: MockWorkspace,
		agent: agentWithApps,
		onRemoveWorkspace: fn(),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const workspaceMenuButton = canvas.getByRole("button", {
			name: `${MockWorkspace.name} workspace menu`,
		});
		expect(
			canvas.queryByRole("button", {
				name: `Remove workspace ${MockWorkspace.name}`,
			}),
		).not.toBeInTheDocument();

		await userEvent.click(workspaceMenuButton);
		let detachWorkspaceItem: HTMLElement | null = null;
		await waitFor(() => {
			const menuId = workspaceMenuButton.getAttribute("aria-controls");
			if (!menuId) {
				throw new Error("Expected workspace pill to control a menu.");
			}

			const menu = canvasElement.ownerDocument.getElementById(menuId);
			if (!(menu instanceof HTMLElement)) {
				throw new Error("Expected workspace menu to render.");
			}

			detachWorkspaceItem = within(menu).getByRole("menuitem", {
				name: "Detach workspace",
			});
			expect(detachWorkspaceItem).toBeVisible();
		});
		if (!detachWorkspaceItem) {
			throw new Error("Expected detach workspace menu item to render.");
		}

		await userEvent.click(detachWorkspaceItem);
		expect(args.onRemoveWorkspace).toHaveBeenCalledTimes(1);
	},
};

export const WithBuiltinAppsOnly: Story = {
	args: {
		...defaultProps,
		workspace: MockWorkspace,
		agent: agentWithBuiltinsOnly,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const pill = canvas.getByText("test-workspace");
		await userEvent.click(pill);

		await waitFor(() => {
			const body = within(document.body);
			expect(body.getByText("VS Code")).toBeInTheDocument();
			expect(body.getByText("Terminal")).toBeInTheDocument();
			expect(body.getByText("View Workspace")).toBeInTheDocument();
			// No external apps or VS Code Insiders.
			expect(body.queryByText("VS Code Insiders")).not.toBeInTheDocument();
		});
	},
};

export const WithExternalAppsOnly: Story = {
	args: {
		...defaultProps,
		workspace: MockWorkspace,
		agent: agentWithExternalOnly,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const pill = canvas.getByText("test-workspace");
		await userEvent.click(pill);

		await waitFor(() => {
			const body = within(document.body);
			expect(body.getByText("JetBrains Gateway")).toBeInTheDocument();
			expect(body.getByText("Cursor")).toBeInTheDocument();
			expect(body.getByText("View Workspace")).toBeInTheDocument();
			// No built-in apps.
			expect(body.queryByText("VS Code")).not.toBeInTheDocument();
			expect(body.queryByText("Terminal")).not.toBeInTheDocument();
		});
	},
};

export const NoApps: Story = {
	args: {
		...defaultProps,
		workspace: MockWorkspace,
		agent: agentWithNoApps,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const pill = canvas.getByText("test-workspace");
		await userEvent.click(pill);

		await waitFor(() => {
			const body = within(document.body);
			expect(body.getByText("View Workspace")).toBeInTheDocument();
		});
	},
};

export const WithHiddenApp: Story = {
	args: {
		...defaultProps,
		workspace: MockWorkspace,
		agent: agentWithHiddenApp,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const pill = canvas.getByText("test-workspace");
		await userEvent.click(pill);

		await waitFor(() => {
			const body = within(document.body);
			// Visible apps should appear.
			expect(body.getByText("VS Code")).toBeInTheDocument();
			expect(body.getByText("JetBrains Gateway")).toBeInTheDocument();
			// Hidden app should NOT appear.
			expect(body.queryByText("Hidden Internal Tool")).not.toBeInTheDocument();
		});
	},
};

export const WithStoppedWorkspace: Story = {
	args: {
		...defaultProps,
		workspace: MockStoppedWorkspace,
		agent: agentWithApps,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const pill = canvas.getByText(MockStoppedWorkspace.name);
		await userEvent.click(pill);

		await waitFor(() => {
			const body = within(document.body);

			// VS Code items should be present but disabled.
			const vscodeItem = body.getByText("VS Code").closest("[role=menuitem]");
			expect(vscodeItem).toHaveAttribute("aria-disabled", "true");

			const vscodeInsidersItem = body
				.getByText("VS Code Insiders")
				.closest("[role=menuitem]");
			expect(vscodeInsidersItem).toHaveAttribute("aria-disabled", "true");

			// Terminal item should be disabled.
			const terminalItem = body
				.getByText("Terminal")
				.closest("[role=menuitem]");
			expect(terminalItem).toHaveAttribute("aria-disabled", "true");

			// External app items should be disabled.
			const jetbrainsItem = body
				.getByText("JetBrains Gateway")
				.closest("[role=menuitem]");
			expect(jetbrainsItem).toHaveAttribute("aria-disabled", "true");
			expect(jetbrainsItem).not.toHaveAttribute("href");

			const cursorItem = body.getByText("Cursor").closest("[role=menuitem]");
			expect(cursorItem).toHaveAttribute("aria-disabled", "true");
			expect(cursorItem).not.toHaveAttribute("href");

			// View Workspace link should still be accessible.
			expect(body.getByText("View Workspace")).toBeInTheDocument();

			// Ports sub-trigger should be disabled when workspace is stopped.
			const portsItem = body.getByText("Ports").closest("[role=menuitem]");
			expect(portsItem).toHaveAttribute("aria-disabled", "true");
		});
	},
};

export const WithListeningPorts: Story = {
	args: {
		...defaultProps,
		workspace: MockWorkspace,
		agent: MockWorkspaceAgent,
	},
	parameters: {
		queries: [
			{ key: ["me", "apiKey"], data: { key: "mock-api-key" } },
			{
				key: ["portForward", MockWorkspaceAgent.id],
				data: MockListeningPortsResponse,
			},
			{
				key: ["sharedPorts", MockWorkspace.id],
				data: { shares: [] },
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const pill = canvas.getByText("test-workspace");
		await userEvent.click(pill);

		const body = within(document.body);
		await waitFor(() => {
			// The ports sub-trigger should show the count.
			expect(body.getByText(/Ports \(\d+\)/)).toBeInTheDocument();
		});

		// Hover over the ports item to open the submenu.
		await userEvent.hover(body.getByText(/Ports \(\d+\)/));

		await waitFor(() => {
			expect(body.getByText("Listening Ports")).toBeInTheDocument();
			expect(body.getByText("8080")).toBeInTheDocument();
			expect(body.getByText("gogo")).toBeInTheDocument();
			expect(body.getByText("30000")).toBeInTheDocument();
			expect(body.getByText("webb")).toBeInTheDocument();
			expect(body.getByText("Manage sharing")).toBeInTheDocument();
			// Port items render as anchor links.
			const port8080Anchor = body.getByText("8080").closest("a");
			expect(port8080Anchor).toHaveAttribute("href");
		});
	},
};

export const WithSharedPorts: Story = {
	args: {
		...defaultProps,
		workspace: MockWorkspace,
		agent: {
			...MockWorkspaceAgent,
			name: "a-workspace-agent",
		},
	},
	parameters: {
		queries: [
			{ key: ["me", "apiKey"], data: { key: "mock-api-key" } },
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const pill = canvas.getByText("test-workspace");
		await userEvent.click(pill);

		const body = within(document.body);
		await waitFor(() => {
			expect(body.getByText(/Ports/)).toBeInTheDocument();
		});

		await userEvent.hover(body.getByText(/Ports/));

		await waitFor(() => {
			expect(body.getByText("Listening Ports")).toBeInTheDocument();
			expect(body.getByText("Shared Ports")).toBeInTheDocument();
			// Shared ports from MockSharedPortsResponse for this agent.
			expect(body.getByText("4000")).toBeInTheDocument();
			expect(body.getByText("Manage sharing")).toBeInTheDocument();
			// Port 8081 is both listening and shared; deduplication ensures it
			// appears only in the Shared Ports section, not in Listening Ports.
			expect(body.getAllByText("8081")).toHaveLength(1);
		});
	},
};

export const EmptyPorts: Story = {
	args: {
		...defaultProps,
		workspace: MockWorkspace,
		agent: MockWorkspaceAgent,
	},
	parameters: {
		queries: [
			{ key: ["me", "apiKey"], data: { key: "mock-api-key" } },
			{
				key: ["portForward", MockWorkspaceAgent.id],
				data: { ports: [] },
			},
			{
				key: ["sharedPorts", MockWorkspace.id],
				data: { shares: [] },
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const pill = canvas.getByText("test-workspace");
		await userEvent.click(pill);

		const body = within(document.body);
		await waitFor(() => {
			expect(body.getByText("Ports (0)")).toBeInTheDocument();
		});

		await userEvent.hover(body.getByText("Ports (0)"));

		await waitFor(() => {
			expect(body.getByText("No open ports detected.")).toBeInTheDocument();
		});
	},
};

const mobilePortsStoryConfig = {
	args: {
		...defaultProps,
		workspace: MockWorkspace,
		agent: {
			...MockWorkspaceAgent,
			name: "a-workspace-agent",
		},
	},
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		chromatic: { viewports: [375] },
		queries: [
			{ key: ["me", "apiKey"], data: { key: "mock-api-key" } },
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
} satisfies Partial<Story>;

const openMobilePortsPanel = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	const pill = await canvas.findByRole("button", {
		name: /workspace menu/,
	});
	await userEvent.click(pill);

	const body = within(document.body);
	const portsItem = await body.findByText(/Ports \(\d+\)/);
	await userEvent.click(portsItem);

	return { body, pill };
};

export const MobilePortsInlinePanel: Story = {
	...mobilePortsStoryConfig,
	parameters: {
		...mobilePortsStoryConfig.parameters,
		// TODO: This story fails when pixel runs its play function. Fix it and remove the exclude.
		pixel: { exclude: true },
	},
	play: async ({ canvasElement }) => {
		const { body, pill } = await openMobilePortsPanel(canvasElement);

		await waitFor(() => {
			expect(body.getByText("Listening Ports")).toBeInTheDocument();
			expect(body.getByText("Shared Ports")).toBeInTheDocument();
			expect(body.getByText("Manage sharing")).toBeInTheDocument();
			expect(body.getByRole("menuitem", { name: /Back/ })).toHaveFocus();
			expect(body.queryByText("View Workspace")).not.toBeInTheDocument();
		});

		const portsHeader = body.getByText("Listening Ports");
		const dropdown: HTMLElement | null = portsHeader.closest(
			"[data-radix-popper-content-wrapper]",
		);
		expect(dropdown).not.toBeNull();
		if (dropdown === null) {
			throw new Error("Expected dropdown wrapper to exist");
		}
		const rect = dropdown.getBoundingClientRect();
		expect(rect.right).toBeLessThanOrEqual(innerWidth);
		expect(rect.left).toBeGreaterThanOrEqual(0);

		await userEvent.click(body.getByRole("menuitem", { name: /Back/ }));
		await waitFor(() => {
			expect(body.getByText("View Workspace")).toBeInTheDocument();
			expect(body.getByRole("menuitem", { name: /Ports/ })).toHaveFocus();
			expect(body.queryByText("Listening Ports")).not.toBeInTheDocument();
		});

		await userEvent.click(body.getByText(/Ports \(\d+\)/));
		await waitFor(() => {
			expect(body.getByText("Listening Ports")).toBeInTheDocument();
			expect(body.getByRole("menuitem", { name: /Back/ })).toHaveFocus();
		});

		await userEvent.keyboard("{Escape}");
		await waitFor(() => {
			expect(body.queryByText("Listening Ports")).not.toBeInTheDocument();
		});

		await userEvent.click(pill);
		await waitFor(() => {
			expect(body.getByText("View Workspace")).toBeInTheDocument();
			expect(body.queryByText("Listening Ports")).not.toBeInTheDocument();
		});
	},
};

export const MobilePortsInlinePanelOpen: Story = {
	...mobilePortsStoryConfig,
	play: async ({ canvasElement }) => {
		const { body } = await openMobilePortsPanel(canvasElement);

		await waitFor(() => {
			expect(body.getByText("Listening Ports")).toBeInTheDocument();
		});
	},
};
