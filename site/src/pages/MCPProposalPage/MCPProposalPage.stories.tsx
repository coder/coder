import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import type {
	AcceptMCPServerProposalResponse,
	MCPServerProposal,
} from "#/api/typesGenerated";
import { mockApiError } from "#/testHelpers/entities";
import MCPProposalPage from "./MCPProposalPage";

const MockProposal: MCPServerProposal = {
	id: "8d3e0a25-6a06-4b3a-9c19-6ba9d0a45a1e",
	chat_id: "b0f5b1c9-9f52-45b3-9a6a-3f2f3b2d4d21",
	status: "pending",
	created_at: "2024-01-01T12:00:00Z",
	display_name: "Linear",
	slug: "linear",
	description: "Access Linear issues and projects from chat.",
	instructions:
		"1. Open [Linear API settings](https://linear.app/settings/api).\n2. Create an API key and copy it.",
	icon_url: "",
	url: "https://mcp.linear.app/mcp",
	transport: "streamable_http",
	auth_type: "oauth2",
	tool_allow_list: ["list_issues", "create_issue"],
	tool_deny_list: ["delete_issue"],
	has_oauth2_client_credentials: true,
	has_api_key: false,
	has_custom_headers: false,
	secret_placeholders: {},
	authenticated: false,
};

const MockConnectUrl =
	"https://dev.coder.com/api/v2/mcp/servers/linear/connect";

const MockAcceptResponse: AcceptMCPServerProposalResponse = {
	mcp_server_config_id: "0f9a9f56-77ee-4a5e-a6c3-1f1e6f9a2b3c",
	authenticated: false,
	connect_url: MockConnectUrl,
};

const meta: Meta<typeof MCPProposalPage> = {
	title: "pages/MCPProposalPage",
	component: MCPProposalPage,
	args: {
		redirectToConnectUrl: fn(),
	},
	parameters: {
		layout: "fullscreen",
		reactRouter: reactRouterParameters({
			location: {
				pathParams: { proposal: MockProposal.id },
			},
			routing: { path: "/agents/settings/mcp-proposals/:proposal" },
		}),
	},
};

export default meta;
type Story = StoryObj<typeof MCPProposalPage>;

export const Pending: Story = {
	beforeEach: () => {
		spyOn(API, "getMCPServerProposal").mockResolvedValue(MockProposal);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByRole("heading", { name: "Linear" });
		await expect(
			canvas.getByRole("heading", { name: "Review MCP server proposal" }),
		).toBeVisible();
		await expect(canvas.getByText("Pending review")).toBeVisible();
		await expect(canvas.getByText("Only you")).toBeVisible();
		await expect(canvas.getByText("linear")).toBeVisible();
		await expect(canvas.getByText("https://mcp.linear.app/mcp")).toBeVisible();
		await expect(canvas.getByText("Streamable HTTP")).toBeVisible();
		await expect(canvas.getByText("OAuth2")).toBeVisible();
		await expect(canvas.getByText("list_issues")).toBeVisible();
		await expect(canvas.getByText("delete_issue")).toBeVisible();
		await expect(canvas.getByText("OAuth2 client credentials")).toBeVisible();
		await expect(
			canvas.getByRole("link", { name: "Linear API settings" }),
		).toHaveAttribute("href", "https://linear.app/settings/api");
		await expect(
			canvas.getByRole("button", { name: "Accept & create server" }),
		).toBeEnabled();
		await expect(canvas.getByRole("button", { name: "Reject" })).toBeEnabled();
	},
};

export const GitHubAPIKeyReview: Story = {
	beforeEach: () => {
		spyOn(API, "getMCPServerProposal").mockResolvedValue({
			...MockProposal,
			created_at: new Date(Date.now() - 4 * 60 * 1_000).toISOString(),
			display_name: "GitHub",
			slug: "github",
			description:
				"Official GitHub MCP server for managing repositories, issues, pull requests, and more.",
			instructions:
				'1. Go to [GitHub token settings](https://github.com/settings/tokens).\n2. Click *"Generate new token"*, then *"Generate new token (classic)"*.\n3. Give it a descriptive name, such as `Coder MCP`.\n4. Select the scopes you need: `repo`, `read:org`, and `read:user`.\n5. Generate and copy the token.\n6. Paste the value in the format `Bearer ghp_yourTokenHere`.',
			url: "https://api.githubcopilot.com/mcp/",
			transport: "streamable_http",
			auth_type: "api_key",
			tool_allow_list: undefined,
			tool_deny_list: undefined,
			has_oauth2_client_credentials: false,
			has_api_key: false,
			secret_placeholders: {
				api_key_value: "Bearer <your-github-personal-access-token>",
			},
		});
	},
};

export const AcceptRedirectsToConnectUrl: Story = {
	beforeEach: () => {
		spyOn(API, "getMCPServerProposal").mockResolvedValue(MockProposal);
		spyOn(API, "acceptMCPServerProposal").mockResolvedValue(MockAcceptResponse);
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const acceptButton = await canvas.findByRole("button", {
			name: "Accept & create server",
		});
		await userEvent.click(acceptButton);
		await waitFor(() => {
			expect(API.acceptMCPServerProposal).toHaveBeenCalledWith(
				MockProposal.id,
				{},
			);
		});
		await waitFor(() => {
			expect(args.redirectToConnectUrl).toHaveBeenCalledWith(MockConnectUrl);
		});
		await canvas.findByText("Redirecting to authentication");
	},
};

export const APIKeyPlaceholder: Story = {
	beforeEach: () => {
		spyOn(API, "getMCPServerProposal").mockResolvedValue({
			...MockProposal,
			auth_type: "api_key",
			has_oauth2_client_credentials: false,
			secret_placeholders: {
				api_key_value: "Paste your Linear API key",
			},
		});
		spyOn(API, "acceptMCPServerProposal").mockResolvedValue({
			mcp_server_config_id: MockAcceptResponse.mcp_server_config_id,
			authenticated: true,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const acceptButton = await canvas.findByRole("button", {
			name: "Accept & create server",
		});
		await expect(acceptButton).toBeDisabled();
		const apiKey = canvas.getByPlaceholderText("Paste your Linear API key");
		const credentials = canvas.getByRole("region", {
			name: "Required credentials",
		});
		await expect(
			canvas.queryByRole("region", { name: "Instructions" }),
		).not.toBeInTheDocument();
		const showAllButton = canvas.getByRole("button", { name: "Show all" });
		await expect(showAllButton).toBeVisible();
		await userEvent.click(showAllButton);
		await expect(
			canvas.queryByRole("button", { name: "Show all" }),
		).not.toBeInTheDocument();
		const instructions = canvas.getByRole("region", {
			name: "Instructions",
		});
		await expect(
			canvas.getByRole("button", { name: "Setup instructions" }),
		).toHaveAttribute("data-state", "open");
		await expect(credentials).toContainElement(instructions);
		await expect(
			within(instructions).getByRole("link", { name: "Linear API settings" }),
		).toBeVisible();
		await expect(apiKey).toHaveAttribute(
			"placeholder",
			"Paste your Linear API key",
		);
		await userEvent.type(apiKey, "linear-secret");
		await expect(acceptButton).toBeEnabled();
		await userEvent.click(acceptButton);
		await waitFor(() => {
			expect(API.acceptMCPServerProposal).toHaveBeenCalledWith(
				MockProposal.id,
				{ api_key_value: "linear-secret" },
			);
		});
	},
};

export const WithoutInstructions: Story = {
	beforeEach: () => {
		spyOn(API, "getMCPServerProposal").mockResolvedValue({
			...MockProposal,
			instructions: undefined,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByRole("heading", { name: "Linear" });
		await expect(
			canvas.queryByRole("region", { name: "Instructions" }),
		).not.toBeInTheDocument();
	},
};

export const CredentialErrorRetainsInput: Story = {
	beforeEach: () => {
		spyOn(API, "getMCPServerProposal").mockResolvedValue({
			...MockProposal,
			auth_type: "api_key",
			has_oauth2_client_credentials: false,
			secret_placeholders: {
				api_key_value: "Paste your Linear API key",
			},
		});
		spyOn(API, "acceptMCPServerProposal").mockRejectedValue(
			mockApiError({ message: "Invalid MCP server proposal credentials." }),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const apiKey = await canvas.findByPlaceholderText(
			"Paste your Linear API key",
		);
		await userEvent.type(apiKey, "keep-this-secret");
		await userEvent.click(
			canvas.getByRole("button", { name: "Accept & create server" }),
		);
		await canvas.findByText("Invalid MCP server proposal credentials.");
		await expect(apiKey).toHaveValue("keep-this-secret");
	},
};

export const OAuth2ClientSecretPlaceholder: Story = {
	beforeEach: () => {
		spyOn(API, "getMCPServerProposal").mockResolvedValue({
			...MockProposal,
			secret_placeholders: {
				oauth2_client_secret: "Paste the registered client secret",
			},
		});
		spyOn(API, "acceptMCPServerProposal").mockResolvedValue(MockAcceptResponse);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const clientSecret = await canvas.findByPlaceholderText(
			"Paste the registered client secret",
		);
		await userEvent.type(clientSecret, "oauth-secret");
		await userEvent.click(
			canvas.getByRole("button", { name: "Accept & create server" }),
		);
		await waitFor(() => {
			expect(API.acceptMCPServerProposal).toHaveBeenCalledWith(
				MockProposal.id,
				{ oauth2_client_secret: "oauth-secret" },
			);
		});
	},
};

export const CustomHeaderPlaceholders: Story = {
	beforeEach: () => {
		spyOn(API, "getMCPServerProposal").mockResolvedValue({
			...MockProposal,
			auth_type: "custom_headers",
			has_oauth2_client_credentials: false,
			secret_placeholders: {
				custom_headers: {
					"X-API-Key": "Paste your API key",
					"X-Account-Token": "Paste your account token",
				},
			},
		});
		spyOn(API, "acceptMCPServerProposal").mockResolvedValue({
			mcp_server_config_id: MockAcceptResponse.mcp_server_config_id,
			authenticated: true,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const acceptButton = await canvas.findByRole("button", {
			name: "Accept & create server",
		});
		await userEvent.type(
			canvas.getByPlaceholderText("Paste your API key"),
			"api-secret",
		);
		await expect(acceptButton).toBeDisabled();
		await userEvent.type(
			canvas.getByPlaceholderText("Paste your account token"),
			"account-secret",
		);
		await expect(acceptButton).toBeEnabled();
		await userEvent.click(acceptButton);
		await waitFor(() => {
			expect(API.acceptMCPServerProposal).toHaveBeenCalledWith(
				MockProposal.id,
				{
					custom_headers: {
						"X-API-Key": "api-secret",
						"X-Account-Token": "account-secret",
					},
				},
			);
		});
	},
};

export const AcceptAlreadyAuthenticated: Story = {
	beforeEach: () => {
		spyOn(API, "getMCPServerProposal").mockResolvedValue(MockProposal);
		spyOn(API, "acceptMCPServerProposal").mockResolvedValue({
			mcp_server_config_id: MockAcceptResponse.mcp_server_config_id,
			authenticated: true,
		});
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const acceptButton = await canvas.findByRole("button", {
			name: "Accept & create server",
		});
		await userEvent.click(acceptButton);
		await canvas.findByText("MCP server created");
		await expect(
			canvas.getByText("The server was created and enabled for the chat."),
		).toBeVisible();
		await expect(args.redirectToConnectUrl).not.toHaveBeenCalled();
	},
};

export const Reject: Story = {
	beforeEach: () => {
		spyOn(API, "getMCPServerProposal").mockResolvedValue(MockProposal);
		spyOn(API, "rejectMCPServerProposal").mockResolvedValue(undefined);
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const rejectButton = await canvas.findByRole("button", {
			name: "Reject",
		});
		await userEvent.click(rejectButton);
		await waitFor(() => {
			expect(API.rejectMCPServerProposal).toHaveBeenCalledWith(MockProposal.id);
		});
		await canvas.findByText("Proposal rejected");
		await expect(args.redirectToConnectUrl).not.toHaveBeenCalled();
	},
};

export const AlreadyAccepted: Story = {
	beforeEach: () => {
		spyOn(API, "getMCPServerProposal").mockResolvedValue({
			...MockProposal,
			status: "accepted",
			mcp_server_config_id: MockAcceptResponse.mcp_server_config_id,
			authenticated: false,
			connect_url: MockConnectUrl,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("MCP server created");
		const connectLink = canvas.getByRole("link", {
			name: "Connect your account",
		});
		await expect(connectLink).toHaveAttribute("href", MockConnectUrl);
	},
};

export const Expired: Story = {
	beforeEach: () => {
		spyOn(API, "getMCPServerProposal").mockRejectedValue({
			...mockApiError({
				message: "This MCP server proposal has expired or was already handled.",
			}),
			status: 404,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("Proposal unavailable");
		await expect(
			canvas.getByText(
				"This MCP server proposal has expired or was already handled.",
			),
		).toBeVisible();
	},
};

export const Forbidden: Story = {
	beforeEach: () => {
		spyOn(API, "getMCPServerProposal").mockRejectedValue({
			...mockApiError({
				message: "Another user must authorize this MCP server proposal.",
			}),
			status: 403,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("Not authorized");
		await expect(
			canvas.getByText("Another user must authorize this MCP server proposal."),
		).toBeVisible();
	},
};

export const FetchErrorWithRetry: Story = {
	beforeEach: () => {
		spyOn(API, "getMCPServerProposal")
			.mockRejectedValueOnce({
				...mockApiError({ message: "Internal server error." }),
				status: 500,
			})
			.mockResolvedValue(MockProposal);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const retryButton = await canvas.findByRole("button", { name: "Retry" });
		await userEvent.click(retryButton);
		await canvas.findByRole("heading", { name: "Linear" });
	},
};
