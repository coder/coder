import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, userEvent } from "storybook/test";
import { mockApiError } from "#/testHelpers/entities";
import { MCPTokenDialog } from "./MCPTokenDialog";
import { builtInTokenHelp } from "./mcpPresets";

const meta: Meta<typeof MCPTokenDialog> = {
	title: "modules/mcp/MCPTokenDialog",
	component: MCPTokenDialog,
	args: {
		serverName: "Slack",
		audience: "Your chats",
		hasCredential: false,
		help: builtInTokenHelp.slack,
		isSaving: false,
		isRemoving: false,
		error: undefined,
		onSave: fn(),
		onRemove: fn(),
		onClose: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof MCPTokenDialog>;

export const AddToken: Story = {};

export const ReplaceToken: Story = { args: { hasCredential: true } };

export const GitHubHelp: Story = {
	args: { serverName: "GitHub", help: builtInTokenHelp.github },
};

export const Removing: Story = {
	args: { hasCredential: true, isRemoving: true },
};

export const Saving: Story = { args: { isSaving: true } };

export const SaveError: Story = {
	args: { error: mockApiError({ message: "Failed to save the token." }) },
};

const mockStoredApp = {
	app_id: 12345,
	installation_id: 67890,
	repositories: ["coder/coder"],
	use_for_git: true,
};

export const GitHubAppChoice: Story = {
	args: {
		serverName: "GitHub",
		help: builtInTokenHelp.github,
		githubApp: { onSave: fn() },
	},
};

export const GitHubAppForm: Story = {
	args: GitHubAppChoice.args,
	play: async () => {
		await userEvent.click(
			await screen.findByRole("radio", { name: "Use a GitHub App" }),
		);
	},
};

export const GitHubAppStored: Story = {
	args: {
		serverName: "GitHub",
		hasCredential: true,
		githubApp: { current: mockStoredApp, onSave: fn() },
	},
};

export const GitHubAppKeyRejected: Story = {
	args: {
		...GitHubAppForm.args,
		error: mockApiError({
			message: "Invalid request.",
			validations: [
				{
					field: "github_app.private_key",
					detail: "The private key is not a valid PEM key.",
				},
			],
		}),
	},
	play: GitHubAppForm.play,
};

export const GitHubAppRejectedByGitHub: Story = {
	args: {
		...GitHubAppForm.args,
		error: mockApiError({
			message: "GitHub rejected the GitHub App credential.",
			detail: "Not Found",
		}),
	},
	play: GitHubAppForm.play,
};
