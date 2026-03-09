import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ChatDiffStatusResponse } from "api/api";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { AgentDetailTopBar } from "./TopBar";

const mockDiffStatus: ChatDiffStatusResponse = {
	chat_id: "chat-1",
	changes_requested: false,
	additions: 42,
	deletions: 7,
	changed_files: 5,
};

const defaultProps = {
	chatTitle: "Build authentication feature",
	onOpenParentChat: () => {},
	diff: {
		hasDiffStatus: false,
		diffStatus: undefined,
		hasGitRepos: false,
		gitRepoCount: 0,
		gitRepositories: new Map(),
		showSidebarPanel: false,
		onToggleSidebar: () => {},
	},
	workspace: {
		canOpenEditors: true,
		canOpenWorkspace: true,
		onOpenInEditor: () => {},
		onViewWorkspace: () => {},
		onOpenTerminal: () => {},
		sshCommand: "ssh main.my-workspace.admin.coder",
	},
	onArchiveAgent: () => {},
	onArchiveAndDeleteWorkspace: () => {},
	onUnarchiveAgent: () => {},
	isSidebarCollapsed: false,
	onToggleSidebarCollapsed: () => {},
} satisfies React.ComponentProps<typeof AgentDetailTopBar>;

const meta: Meta<typeof AgentDetailTopBar> = {
	title: "pages/AgentsPage/AgentDetail/TopBar",
	component: AgentDetailTopBar,
	parameters: {
		layout: "fullscreen",
	},
	args: defaultProps,
};
export default meta;
type Story = StoryObj<typeof AgentDetailTopBar>;

export const Default: Story = {};

export const WithDiffStats: Story = {
	args: {
		diff: {
			hasDiffStatus: true,
			diffStatus: mockDiffStatus,
			hasGitRepos: false,
			gitRepoCount: 0,
			gitRepositories: new Map(),
			showSidebarPanel: false,
			onToggleSidebar: () => {},
		},
	},
};

export const WithDiffPanelOpen: Story = {
	args: {
		diff: {
			hasDiffStatus: true,
			diffStatus: mockDiffStatus,
			hasGitRepos: false,
			gitRepoCount: 0,
			gitRepositories: new Map(),
			showSidebarPanel: true,
			onToggleSidebar: () => {},
		},
	},
};

export const WithParentChat: Story = {
	args: {
		parentChat: {
			id: "parent-chat-1",
			owner_id: "owner-id",
			last_model_config_id: "model-config-1",
			title: "Set up CI/CD pipeline",
			status: "completed",
			last_error: null,
			created_at: "2026-02-18T00:00:00.000Z",
			updated_at: "2026-02-18T00:00:00.000Z",
			archived: false,
		},
	},
};

export const SidebarCollapsed: Story = {
	args: {
		isSidebarCollapsed: true,
	},
};

export const Archived: Story = {
	args: {
		isArchived: true,
	},
};

export const NoTitle: Story = {
	args: {
		chatTitle: undefined,
	},
};

export const ArchivedWithUnarchive: Story = {
	args: {
		isArchived: true,
		onUnarchiveAgent: () => {},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Open the actions dropdown
		const trigger = canvas.getByLabelText("Open agent actions");
		await userEvent.click(trigger);
		// Verify "Unarchive Agent" is shown instead of "Archive Agent"
		await waitFor(() => {
			const body = within(document.body);
			expect(body.getByText("Unarchive Agent")).toBeInTheDocument();
		});
		const body = within(document.body);
		expect(body.queryByText("Archive Agent")).not.toBeInTheDocument();
		expect(
			body.queryByText("Archive & Delete Workspace"),
		).not.toBeInTheDocument();
	},
};
