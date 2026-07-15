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
	icon_url: "",
	url: "https://mcp.linear.app/mcp",
	transport: "streamable_http",
	auth_type: "oauth2",
	tool_allow_list: ["list_issues", "create_issue"],
	tool_deny_list: ["delete_issue"],
	has_oauth2_client_credentials: true,
	has_api_key: false,
	has_custom_headers: false,
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
			routing: { path: "/mcp-proposals/:proposal" },
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
			canvas.getByText("This MCP server will only be available to you."),
		).toBeVisible();
		await expect(canvas.getByText("linear")).toBeVisible();
		await expect(canvas.getByText("https://mcp.linear.app/mcp")).toBeVisible();
		await expect(canvas.getByText("Streamable HTTP")).toBeVisible();
		await expect(canvas.getByText("OAuth2")).toBeVisible();
		await expect(canvas.getByText("list_issues")).toBeVisible();
		await expect(canvas.getByText("delete_issue")).toBeVisible();
		await expect(canvas.getByText("OAuth2 client credentials")).toBeVisible();
		await expect(canvas.getByRole("button", { name: "Accept" })).toBeEnabled();
		await expect(canvas.getByRole("button", { name: "Reject" })).toBeEnabled();
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
			name: "Accept",
		});
		await userEvent.click(acceptButton);
		await waitFor(() => {
			expect(API.acceptMCPServerProposal).toHaveBeenCalledWith(MockProposal.id);
		});
		await waitFor(() => {
			expect(args.redirectToConnectUrl).toHaveBeenCalledWith(MockConnectUrl);
		});
		await canvas.findByText("Redirecting to authentication");
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
			name: "Accept",
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
