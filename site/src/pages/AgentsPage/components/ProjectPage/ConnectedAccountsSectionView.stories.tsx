import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type { ChatProjectMCPServerToken } from "#/api/typesGenerated";
import {
	MockChatProjectExternalAuth,
	MockChatProjectGitHubMCPServerToken,
	MockChatProjectMCPServerToken,
	MockGithubAuthLink,
	mockApiError,
} from "#/testHelpers/entities";
import { ConnectedAccountsSectionView } from "./ConnectedAccountsSection";

const mockGitHubTokenStored: ChatProjectMCPServerToken = {
	...MockChatProjectGitHubMCPServerToken,
	has_token: true,
	token_updated_at: "2026-09-04T12:00:00Z",
};

const meta: Meta<typeof ConnectedAccountsSectionView> = {
	title: "pages/AgentsPage/ProjectPage/ConnectedAccountsSectionView",
	component: ConnectedAccountsSectionView,
	decorators: [
		(Story) => (
			<div className="@container max-w-4xl">
				<Story />
			</div>
		),
	],
	args: {
		auths: MockChatProjectExternalAuth,
		error: undefined,
		onRetry: fn(),
		mcpTokens: [],
		canManageAccounts: true,
		builtInServers: new Map([["mcp-github", "github" as const]]),
		onEditMcpToken: fn(),
		connectingProviderId: undefined,
		onConnect: fn(),
		onDisconnect: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ConnectedAccountsSectionView>;

export const Default: Story = {};

export const ReadOnly: Story = { args: { canManageAccounts: false } };

export const Connecting: Story = { args: { connectingProviderId: "gitlab" } };

export const NeedsReconnecting: Story = {
	args: {
		auths: {
			...MockChatProjectExternalAuth,
			links: [
				{
					...MockGithubAuthLink,
					authenticated: false,
					validate_error: "The token was revoked.",
				},
			],
		},
	},
};

export const NoProviders: Story = {
	args: { auths: { providers: [], links: [] } },
};

export const Loading: Story = { args: { auths: undefined } };

export const LoadError: Story = {
	args: {
		auths: undefined,
		error: mockApiError({ message: "Failed to load connected accounts." }),
	},
};

export const WithMCPTokens: Story = {
	args: { mcpTokens: [MockChatProjectMCPServerToken, mockGitHubTokenStored] },
};

export const MCPTokensReadOnly: Story = {
	args: {
		mcpTokens: [MockChatProjectMCPServerToken, mockGitHubTokenStored],
		canManageAccounts: false,
	},
};

export const WithGitHubAppCredential: Story = {
	args: {
		mcpTokens: [
			{
				...mockGitHubTokenStored,
				credential_kind: "github_app",
				github_app: {
					app_id: 12345,
					installation_id: 67890,
					repositories: ["coder/coder", "docs"],
					use_for_git: true,
				},
			},
		],
	},
};

export const GitHubAppDetailsHidden: Story = {
	args: {
		mcpTokens: [
			{
				...mockGitHubTokenStored,
				credential_kind: "github_app",
				github_app: undefined,
			},
		],
		canManageAccounts: false,
	},
};
