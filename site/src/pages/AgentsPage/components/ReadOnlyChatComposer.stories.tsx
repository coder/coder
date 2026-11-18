import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import {
	MockChat,
	MockChatContextClean,
	MockChatQueuedMessage,
	MockMCPServerConfig,
} from "#/testHelpers/chatEntities";
import { MockPersonalModelOptions } from "#/testHelpers/chatModels";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import {
	withDashboardProvider,
	withProxyProvider,
} from "#/testHelpers/storybook";
import { ReadOnlyChatComposer } from "./ReadOnlyChatComposer";

const mockChat = { ...MockChat, owner_name: "Tyler" };
const mockModel = {
	...MockPersonalModelOptions[0],
	model: "gpt-4o",
	displayName: "GPT-4o",
};
const mockLinearMCP = {
	...MockMCPServerConfig,
	id: "mcp-linear",
	display_name: "Linear",
	slug: "linear",
};
const mockGitHubMCP = {
	...MockMCPServerConfig,
	id: "mcp-github",
	display_name: "GitHub",
	slug: "github",
	icon_url: "/icon/github.svg",
	auth_type: "oauth2",
	auth_connected: true,
};

const meta: Meta<typeof ReadOnlyChatComposer> = {
	title: "pages/AgentsPage/ReadOnlyChatComposer",
	component: ReadOnlyChatComposer,
	decorators: [withDashboardProvider, withProxyProvider()],
	args: {
		chat: mockChat,
		model: mockModel,
		modelSelectorPlaceholder: "Select model",
		isModelCatalogLoading: false,
		queuedMessages: [],
		queuedMessageUnderEditID: null,
		isChatPaused: false,
		mcpServers: [mockLinearMCP, mockGitHubMCP],
		selectedMCPServerIds: [mockLinearMCP.id, mockGitHubMCP.id],
		canConfigureAgentSetup: false,
		unsupportedProviderNames: [],
	},
};

export default meta;
type Story = StoryObj<typeof ReadOnlyChatComposer>;

export const Default: Story = {};

export const MCPMenuOpen: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "2 MCPs" }),
		);
	},
};

export const NoActiveMCPServers: Story = {
	args: { selectedMCPServerIds: [] },
};

export const LoadingModel: Story = {
	args: { model: undefined, isModelCatalogLoading: true },
};

export const UnavailableModel: Story = {
	args: { model: undefined, modelSelectorPlaceholder: "Model unavailable" },
};

export const WithWorkspaceAndContext: Story = {
	args: {
		chat: { ...mockChat, plan_mode: "plan" },
		workspace: MockWorkspace,
		workspaceAgent: MockWorkspaceAgent,
		contextUsage: {
			usedTokens: 32_000,
			contextLimitTokens: 128_000,
			context: MockChatContextClean,
		},
	},
};

export const MobileOverflow: Story = {
	args: WithWorkspaceAndContext.args,
	decorators: [
		(Story) => (
			<div className="w-80 max-w-full">
				<Story />
			</div>
		),
	],
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		pixel: { matrix: { viewports: ["phone"] } },
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			await within(canvasElement).findByRole("button", { name: /more item/ }),
		);
	},
};

export const MobileMCPMenuOpen: Story = {
	args: MobileOverflow.args,
	decorators: MobileOverflow.decorators,
	parameters: MobileOverflow.parameters,
	play: async ({ canvasElement }) => {
		await userEvent.click(
			await within(canvasElement).findByRole("button", { name: /more item/ }),
		);
		await userEvent.click(
			await screen.findByRole("button", { name: "2 MCPs" }),
		);
	},
};

export const WithQueuedMessages: Story = {
	args: { queuedMessages: [MockChatQueuedMessage] },
};

export const NoModels: Story = {
	args: { model: undefined, modelCount: 0 },
};
