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
		appExperimentEnabled: true,
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByLabelText("Add panel"));

		const body = within(document.body);
		await waitFor(() => {
			expect(body.getByText("New Terminal")).toBeInTheDocument();
			expect(body.getByText("Preview")).toBeInTheDocument();
			expect(body.getByText("Claude Code")).toBeInTheDocument();
			expect(body.getByText("Documentation")).toBeInTheDocument();
			expect(body.getByText("Ports (3)")).toBeInTheDocument();
		});

		await userEvent.hover(body.getByText("Ports (3)"));
		await waitFor(() => {
			expect(body.getByText("Listening Ports")).toBeInTheDocument();
			expect(body.getByText("8080")).toBeInTheDocument();
		});
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
};

export const AppExperimentDisabled: Story = {
	args: {
		appExperimentEnabled: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByLabelText("New terminal tab")).toBeEnabled();
		await expect(canvas.queryByLabelText("Add panel")).not.toBeInTheDocument();
	},
};
