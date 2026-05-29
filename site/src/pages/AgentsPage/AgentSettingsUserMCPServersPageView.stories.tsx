import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import {
	AgentSettingsUserMCPServersPageView,
	type AgentSettingsUserMCPServersPageViewProps,
} from "./AgentSettingsUserMCPServersPageView";

const now = "2026-03-19T12:00:00.000Z";

const createServerConfig = (
	overrides: Partial<TypesGen.MCPServerConfig> &
		Pick<TypesGen.MCPServerConfig, "id" | "display_name" | "slug">,
): TypesGen.MCPServerConfig => ({
	id: overrides.id,
	display_name: overrides.display_name,
	slug: overrides.slug,
	description: overrides.description ?? "",
	icon_url: overrides.icon_url ?? "",
	transport: overrides.transport ?? "streamable_http",
	url: overrides.url ?? "https://mcp.example.com/sse",
	auth_type: overrides.auth_type ?? "none",
	oauth2_client_id: overrides.oauth2_client_id,
	has_oauth2_secret: overrides.has_oauth2_secret ?? false,
	oauth2_auth_url: overrides.oauth2_auth_url,
	oauth2_token_url: overrides.oauth2_token_url,
	oauth2_scopes: overrides.oauth2_scopes,
	api_key_header: overrides.api_key_header,
	has_api_key: overrides.has_api_key ?? false,
	has_custom_headers: overrides.has_custom_headers ?? false,
	custom_headers_user_keys: overrides.custom_headers_user_keys ?? [],
	tool_allow_list: overrides.tool_allow_list ?? [],
	tool_deny_list: overrides.tool_deny_list ?? [],
	availability: overrides.availability ?? "default_on",
	enabled: overrides.enabled ?? true,
	model_intent: overrides.model_intent ?? false,
	allow_in_plan_mode: overrides.allow_in_plan_mode ?? false,
	forward_coder_headers: overrides.forward_coder_headers ?? false,
	created_at: overrides.created_at ?? now,
	updated_at: overrides.updated_at ?? now,
	auth_connected: overrides.auth_connected ?? false,
});

const meta: Meta<typeof AgentSettingsUserMCPServersPageView> = {
	title: "pages/AgentsPage/AgentSettingsUserMCPServersPageView",
	component: AgentSettingsUserMCPServersPageView,
	args: {
		servers: [],
		isLoadingServers: false,
		serversError: null,
		headerValueStatus: {},
		loadingHeaderStatusIds: new Set(),
		onConnectOAuth2: fn(),
		onSaveHeaderValues: fn(async () => undefined),
		onClearHeaderValues: fn(async () => undefined),
		isSavingHeaderValues: false,
		isClearingHeaderValues: false,
		saveHeaderValuesError: null,
	} satisfies AgentSettingsUserMCPServersPageViewProps,
};

export default meta;
type Story = StoryObj<typeof AgentSettingsUserMCPServersPageView>;

// ── Stories ────────────────────────────────────────────────────

export const NoServersRequireAction: Story = {
	args: {
		servers: [
			createServerConfig({
				id: "mcp-none",
				display_name: "Quiet Server",
				slug: "quiet",
				auth_type: "none",
			}),
		],
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await waitFor(() => {
			expect(
				body.getByText(/No MCP servers require your action/i),
			).toBeInTheDocument();
		});
	},
};

export const Loading: Story = {
	args: {
		servers: undefined,
		isLoadingServers: true,
	},
};

export const ServersError: Story = {
	args: {
		servers: undefined,
		serversError: new Error("Boom"),
	},
};

export const OAuth2NeedsConnect: Story = {
	args: {
		servers: [
			createServerConfig({
				id: "mcp-github",
				display_name: "GitHub MCP",
				slug: "github",
				auth_type: "oauth2",
				auth_connected: false,
			}),
		],
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("button", { name: /Connect/i }),
		);
		expect(args.onConnectOAuth2).toHaveBeenCalledTimes(1);
	},
};

export const OAuth2Connected: Story = {
	args: {
		servers: [
			createServerConfig({
				id: "mcp-github",
				display_name: "GitHub MCP",
				slug: "github",
				auth_type: "oauth2",
				auth_connected: true,
			}),
		],
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await waitFor(() => {
			expect(body.getByText(/Signed in/i)).toBeInTheDocument();
		});
	},
};

export const CustomHeadersNotConfigured: Story = {
	args: {
		servers: [
			createServerConfig({
				id: "mcp-honcho",
				display_name: "Honcho",
				slug: "honcho",
				auth_type: "custom_headers",
				has_custom_headers: true,
				custom_headers_user_keys: ["X-Honcho-User"],
				auth_connected: false,
			}),
		],
		headerValueStatus: {
			"mcp-honcho": { "X-Honcho-User": false },
		},
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await waitFor(() => {
			expect(body.getByText(/Action required/i)).toBeInTheDocument();
		});
	},
};

export const CustomHeadersAllSet: Story = {
	args: {
		servers: [
			createServerConfig({
				id: "mcp-honcho",
				display_name: "Honcho",
				slug: "honcho",
				auth_type: "custom_headers",
				has_custom_headers: true,
				custom_headers_user_keys: ["X-Honcho-User"],
				auth_connected: true,
			}),
		],
		headerValueStatus: {
			"mcp-honcho": { "X-Honcho-User": true },
		},
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await waitFor(() => {
			expect(body.getByText(/Connected/i)).toBeInTheDocument();
		});
	},
};

export const ConfigureSavesValues: Story = {
	args: {
		servers: [
			createServerConfig({
				id: "mcp-honcho",
				display_name: "Honcho",
				slug: "honcho",
				auth_type: "custom_headers",
				has_custom_headers: true,
				custom_headers_user_keys: ["X-Honcho-User", "X-Honcho-Session"],
				auth_connected: false,
			}),
		],
		headerValueStatus: {
			"mcp-honcho": { "X-Honcho-User": false, "X-Honcho-Session": false },
		},
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			await body.findByRole("button", { name: /Configure/i }),
		);

		const userInput = await body.findByLabelText(/X-Honcho-User value/i);
		await userEvent.type(userInput, "user-jwt-abc");

		const sessionInput = body.getByLabelText(/X-Honcho-Session value/i);
		await userEvent.type(sessionInput, "session-jwt-def");

		await userEvent.click(body.getByRole("button", { name: /^Save$/i }));

		await waitFor(() => {
			expect(args.onSaveHeaderValues).toHaveBeenCalledTimes(1);
		});
		const call = (args.onSaveHeaderValues as ReturnType<typeof fn>).mock
			.calls[0];
		expect(call?.[1]).toEqual({
			"X-Honcho-User": "user-jwt-abc",
			"X-Honcho-Session": "session-jwt-def",
		});
	},
};

export const ConfigureClearsValues: Story = {
	args: {
		servers: [
			createServerConfig({
				id: "mcp-honcho",
				display_name: "Honcho",
				slug: "honcho",
				auth_type: "custom_headers",
				has_custom_headers: true,
				custom_headers_user_keys: ["X-Honcho-User"],
				auth_connected: true,
			}),
		],
		headerValueStatus: {
			"mcp-honcho": { "X-Honcho-User": true },
		},
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(await body.findByRole("button", { name: /Edit/i }));

		// Clear button appears because at least one value is set.
		const clearButton = await body.findByRole("button", { name: /Clear all/i });
		await userEvent.click(clearButton);

		await waitFor(() => {
			expect(args.onClearHeaderValues).toHaveBeenCalledTimes(1);
		});
	},
};

export const ConfigureSaveError: Story = {
	args: {
		servers: [
			createServerConfig({
				id: "mcp-honcho",
				display_name: "Honcho",
				slug: "honcho",
				auth_type: "custom_headers",
				has_custom_headers: true,
				custom_headers_user_keys: ["X-Honcho-User"],
				auth_connected: false,
			}),
		],
		headerValueStatus: {
			"mcp-honcho": { "X-Honcho-User": false },
		},
		saveHeaderValuesError: "Server returned 500",
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("button", { name: /Configure/i }),
		);
		await waitFor(() => {
			expect(body.getByText(/Server returned 500/i)).toBeInTheDocument();
		});
	},
};
