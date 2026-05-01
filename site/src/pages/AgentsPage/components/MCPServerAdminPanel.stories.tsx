import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { MCPServerAdminPanel } from "./MCPServerAdminPanel";

// ── Helpers ────────────────────────────────────────────────────

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
	tool_allow_list: overrides.tool_allow_list ?? [],
	tool_deny_list: overrides.tool_deny_list ?? [],
	availability: overrides.availability ?? "default_on",
	enabled: overrides.enabled ?? true,
	model_intent: overrides.model_intent ?? false,
	allow_in_plan_mode: overrides.allow_in_plan_mode ?? false,
	created_at: overrides.created_at ?? now,
	updated_at: overrides.updated_at ?? now,
	auth_connected: overrides.auth_connected ?? false,
});

// ── Meta ───────────────────────────────────────────────────────

const meta: Meta<typeof MCPServerAdminPanel> = {
	title: "pages/AgentsPage/MCPServerAdminPanel",
	component: MCPServerAdminPanel,
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents/settings/mcp-servers" },
			routing: { path: "/agents/settings/mcp-servers" },
		}),
	},
	args: {
		serversData: [],
		isLoadingServers: false,
		serversError: null,
		onCreateServer: fn(async () => ({}) as TypesGen.MCPServerConfig),
		onUpdateServer: fn(async () => ({}) as TypesGen.MCPServerConfig),
		onDeleteServer: fn(async () => undefined),
		isCreatingServer: false,
		isUpdatingServer: false,
		isDeletingServer: false,
		createError: null,
		updateError: null,
		deleteError: null,
	},
};

export default meta;
type Story = StoryObj<typeof MCPServerAdminPanel>;

// ── Stories ────────────────────────────────────────────────────

/** Empty state with no servers configured. */
export const EmptyState: Story = {
	args: {
		serversData: [],
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await expect(
			await body.findByText(/No MCP servers configured yet/i),
		).toBeInTheDocument();

		// Both the section header and the empty state render a distinct
		// Add button, and the empty-state one is the primary CTA.
		expect(
			body.getByRole("button", { name: "Add server" }),
		).toBeInTheDocument();
		expect(
			body.getByRole("button", { name: "Add your first server" }),
		).toBeInTheDocument();
	},
};

/** List view with multiple servers showing status indicators. */
export const ServerList: Story = {
	args: {
		serversData: [
			createServerConfig({
				id: "mcp-sentry",
				display_name: "Sentry",
				slug: "sentry",
				icon_url: "/icon/widgets.svg",
				url: "https://mcp.sentry.io/sse",
				transport: "sse",
				auth_type: "oauth2",
				has_oauth2_secret: true,
				availability: "force_on",
				enabled: true,
			}),
			createServerConfig({
				id: "mcp-linear",
				display_name: "Linear",
				slug: "linear",
				url: "https://mcp.linear.app/v1",
				transport: "streamable_http",
				auth_type: "api_key",
				has_api_key: true,
				availability: "default_on",
				enabled: true,
			}),
			createServerConfig({
				id: "mcp-github",
				display_name: "GitHub",
				slug: "github",
				icon_url: "/icon/github.svg",
				url: "https://api.githubcopilot.com/mcp/",
				transport: "streamable_http",
				auth_type: "oauth2",
				has_oauth2_secret: true,
				availability: "default_off",
				enabled: false,
			}),
		],
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		// All three servers should be visible.
		await expect(
			await body.findByRole("button", { name: /Sentry/ }),
		).toBeInTheDocument();
		expect(body.getByRole("button", { name: /Linear/ })).toBeInTheDocument();
		expect(body.getByRole("button", { name: /GitHub/ })).toBeInTheDocument();

		// Disabled servers surface a warning badge next to the name.
		expect(body.getByText(/^disabled$/i)).toBeInTheDocument();
	},
};

/** Navigate to the create form and fill it out. */
export const CreateServer: Story = {
	args: {
		serversData: [],
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Click Add Server.
		await userEvent.click(
			await body.findByRole("button", { name: /Add your first server/i }),
		);

		// Fill in the Display name field.
		const nameInput = await body.findByLabelText(/Display Name/i);
		await userEvent.type(nameInput, "Sentry");

		// Required fields (display name, slug, server URL) are always visible.
		// Optional sections start collapsed and the Enabled switch is edit-only.
		expect(body.getByLabelText(/^Slug/i)).toBeInTheDocument();
		expect(body.getByLabelText(/Server URL/i)).toBeInTheDocument();
		expect(body.queryByLabelText(/Description/i)).not.toBeInTheDocument();
		expect(
			body.queryByRole("switch", { name: /Enabled/i }),
		).not.toBeInTheDocument();

		// Slug should auto-populate from the display name.
		await expect(body.getByLabelText(/^Slug/i)).toHaveValue("sentry");

		await userEvent.type(
			body.getByLabelText(/Server URL/i),
			"https://mcp.sentry.io/sse",
		);

		// Submit.
		await userEvent.click(body.getByRole("button", { name: /Create server/i }));

		await waitFor(() => {
			expect(args.onCreateServer).toHaveBeenCalledTimes(1);
		});
		expect(args.onCreateServer).toHaveBeenCalledWith(
			expect.objectContaining({
				display_name: "Sentry",
				slug: "sentry",
				url: "https://mcp.sentry.io/sse",
				transport: "streamable_http",
				auth_type: "none",
			}),
		);
	},
};

/** Open the create form and select OAuth2 auth type. */
export const CreateServerOAuth2: Story = {
	args: {
		serversData: [],
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			await body.findByRole("button", { name: /Add your first server/i }),
		);

		await userEvent.type(await body.findByLabelText(/Display Name/i), "GitHub");
		await userEvent.type(
			body.getByLabelText(/Server URL/i),
			"https://api.githubcopilot.com/mcp/",
		);

		await userEvent.click(
			await body.findByRole("button", { name: /Authentication/i }),
		);
		// Select OAuth2 from the Radix Select dropdown.
		await userEvent.click(body.getByLabelText(/Authentication/i));
		await userEvent.click(await body.findByRole("option", { name: /OAuth2/i }));

		// OAuth2 fields should appear.
		await expect(await body.findByLabelText(/Client ID/i)).toBeInTheDocument();
		expect(body.getByLabelText(/Client Secret/i)).toBeInTheDocument();
		expect(body.getByLabelText(/Authorization URL/i)).toBeInTheDocument();
		expect(body.getByLabelText(/Token URL/i)).toBeInTheDocument();
		expect(body.getByLabelText(/^Scopes/i)).toBeInTheDocument();

		// Fill OAuth2 fields.
		await userEvent.type(body.getByLabelText(/Client ID/i), "my-client-id");
		await userEvent.type(body.getByLabelText(/Client Secret/i), "my-secret");

		// Submit.
		await userEvent.click(body.getByRole("button", { name: /Create server/i }));

		await waitFor(() => {
			expect(args.onCreateServer).toHaveBeenCalledTimes(1);
		});
		expect(args.onCreateServer).toHaveBeenCalledWith(
			expect.objectContaining({
				auth_type: "oauth2",
				oauth2_client_id: "my-client-id",
				oauth2_client_secret: "my-secret",
			}),
		);
	},
};

/** Open the create form and select API Key auth type. */
export const CreateServerAPIKey: Story = {
	args: {
		serversData: [],
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			await body.findByRole("button", { name: /Add your first server/i }),
		);

		await userEvent.type(await body.findByLabelText(/Display Name/i), "Linear");
		await userEvent.type(
			body.getByLabelText(/Server URL/i),
			"https://mcp.linear.app/v1",
		);

		await userEvent.click(
			await body.findByRole("button", { name: /Authentication/i }),
		);
		// Select API Key from the Radix Select dropdown.
		await userEvent.click(body.getByLabelText(/Authentication/i));
		await userEvent.click(
			await body.findByRole("option", { name: /API Key/i }),
		);

		// API key fields should appear.
		await expect(
			await body.findByLabelText(/Header Name/i),
		).toBeInTheDocument();
		expect(body.getByLabelText(/API Key/i)).toBeInTheDocument();

		await userEvent.type(body.getByLabelText(/Header Name/i), "Authorization");
		await userEvent.type(body.getByLabelText(/API Key/i), "lin_api_12345");

		await userEvent.click(body.getByRole("button", { name: /Create server/i }));

		await waitFor(() => {
			expect(args.onCreateServer).toHaveBeenCalledTimes(1);
		});
		expect(args.onCreateServer).toHaveBeenCalledWith(
			expect.objectContaining({
				auth_type: "api_key",
				api_key_header: "Authorization",
				api_key_value: "lin_api_12345",
			}),
		);
	},
};

/** Click an existing server to open the edit form. */
export const EditServer: Story = {
	args: {
		serversData: [
			createServerConfig({
				id: "mcp-sentry",
				display_name: "Sentry",
				slug: "sentry",
				description: "Error tracking",
				url: "https://mcp.sentry.io/sse",
				transport: "sse",
				auth_type: "none",
				availability: "default_on",
				enabled: true,
			}),
		],
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Click the server row.
		await userEvent.click(await body.findByRole("button", { name: /Sentry/ }));

		// The inline name input should be pre-populated.
		const nameInput = await body.findByLabelText(/Display Name/i);
		expect(nameInput).toHaveValue("Sentry");

		// Slug and Server URL are always visible.
		expect(body.getByLabelText(/^Slug/i)).toHaveValue("sentry");
		expect(body.getByLabelText(/Server URL/i)).toHaveValue(
			"https://mcp.sentry.io/sse",
		);

		// Expand Details to reach the description field.
		await userEvent.click(body.getByRole("button", { name: /Details/i }));

		// Update the description.
		const descField = body.getByLabelText(/Description/i);
		await userEvent.clear(descField);
		await userEvent.type(descField, "Sentry error tracking integration");

		await userEvent.click(body.getByRole("button", { name: /Save changes/i }));

		await waitFor(() => {
			expect(args.onUpdateServer).toHaveBeenCalledTimes(1);
		});
		expect(args.onUpdateServer).toHaveBeenCalledWith({
			id: "mcp-sentry",
			req: expect.objectContaining({
				description: "Sentry error tracking integration",
			}),
		});
	},
};

/** Edit a server that has OAuth2 — secret field should show placeholder. */
export const EditServerWithOAuth2Secret: Story = {
	args: {
		serversData: [
			createServerConfig({
				id: "mcp-github",
				display_name: "GitHub",
				slug: "github",
				url: "https://api.githubcopilot.com/mcp/",
				auth_type: "oauth2",
				oauth2_client_id: "gh-client-id",
				has_oauth2_secret: true,
				oauth2_auth_url: "https://github.com/login/oauth/authorize",
				oauth2_token_url: "https://github.com/login/oauth/access_token",
				oauth2_scopes: "repo user",
				availability: "default_on",
				enabled: true,
			}),
		],
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(await body.findByRole("button", { name: /GitHub/ }));

		// Authentication section is collapsed by default.
		await userEvent.click(
			await body.findByRole("button", { name: /Authentication/i }),
		);

		// The OAuth2 fields should be visible.
		const secretField = await body.findByLabelText(/Client Secret/i);
		expect(secretField).toHaveValue("••••••••••••••••");
		expect(body.getByLabelText(/Client ID/i)).toHaveValue("gh-client-id");
	},
};

/** Edit a server that has custom headers configured. */
export const EditServerWithCustomHeaders: Story = {
	args: {
		serversData: [
			createServerConfig({
				id: "mcp-custom",
				display_name: "Custom API",
				slug: "custom-api",
				url: "https://mcp.example.com/v1",
				auth_type: "custom_headers",
				has_custom_headers: true,
				availability: "default_on",
				enabled: true,
			}),
		],
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			await body.findByRole("button", { name: /Custom API/ }),
		);

		// Authentication section is collapsed by default.
		await userEvent.click(
			await body.findByRole("button", { name: /Authentication/i }),
		);

		// Should show message about existing headers.
		await expect(
			await body.findByText(/has custom headers configured/i),
		).toBeInTheDocument();

		// Add a new header.
		await userEvent.click(body.getByRole("button", { name: /Add header/i }));

		await userEvent.type(
			body.getByLabelText(/Header 1 name/i),
			"Authorization",
		);
		await userEvent.type(
			body.getByLabelText(/Header 1 value/i),
			"Bearer tok_abc",
		);

		// Submit.
		await userEvent.click(body.getByRole("button", { name: /Save changes/i }));

		await waitFor(() => {
			expect(args.onUpdateServer).toHaveBeenCalledTimes(1);
		});
		expect(args.onUpdateServer).toHaveBeenCalledWith({
			id: "mcp-custom",
			req: expect.objectContaining({
				custom_headers: { Authorization: "Bearer tok_abc" },
			}),
		});
	},
};

/** Delete a server shows confirmation dialog. */
export const DeleteServerConfirmation: Story = {
	args: {
		serversData: [
			createServerConfig({
				id: "mcp-sentry",
				display_name: "Sentry",
				slug: "sentry",
			}),
		],
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(await body.findByRole("button", { name: /Sentry/ }));

		// Click Delete.
		await userEvent.click(await body.findByRole("button", { name: "Delete" }));

		// Confirmation dialog should appear.
		await expect(
			await body.findByText(/Are you sure you want to delete this MCP server/i),
		).toBeInTheDocument();
		await expect(body.getByRole("dialog")).toBeInTheDocument();
	},
};

/** Cancel delete closes the dialog. */
export const DeleteServerCancelled: Story = {
	args: {
		serversData: [
			createServerConfig({
				id: "mcp-sentry",
				display_name: "Sentry",
				slug: "sentry",
			}),
		],
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(await body.findByRole("button", { name: /Sentry/ }));
		await userEvent.click(await body.findByRole("button", { name: "Delete" }));
		await body.findByText(/Are you sure you want to delete this MCP server/i);
		await userEvent.click(body.getByRole("button", { name: "Cancel" }));

		// The dialog should be closed and the form footer restored.
		await waitFor(() => {
			expect(body.queryByRole("dialog")).not.toBeInTheDocument();
		});
		await expect(
			body.findByRole("button", { name: "Delete" }),
		).resolves.toBeInTheDocument();
		expect(
			body.getByRole("button", { name: /Save changes/i }),
		).toBeInTheDocument();
	},
};

/** Confirm delete in dialog calls the API. */
export const DirectEditWhileLoading: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/agents/settings/mcp-servers",
				searchParams: { server: "mcp-sentry" },
			},
			routing: { path: "/agents/settings/mcp-servers" },
		}),
	},
	args: {
		serversData: undefined,
		isLoadingServers: true,
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		await expect(await body.findByText(/^Loading$/i)).toBeInTheDocument();
		expect(body.queryByLabelText(/Display Name/i)).not.toBeInTheDocument();
		expect(
			body.queryByRole("button", { name: /Save changes/i }),
		).not.toBeInTheDocument();
	},
};

export const DeleteServerConfirmed: Story = {
	args: {
		serversData: [
			createServerConfig({
				id: "mcp-sentry",
				display_name: "Sentry",
				slug: "sentry",
			}),
		],
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(await body.findByRole("button", { name: /Sentry/ }));
		await userEvent.click(await body.findByRole("button", { name: "Delete" }));
		await body.findByText(/Are you sure you want to delete this MCP server/i);
		await userEvent.click(
			body.getByRole("button", { name: /Delete MCP server/i }),
		);

		await waitFor(() => {
			expect(args.onDeleteServer).toHaveBeenCalledTimes(1);
		});
		expect(args.onDeleteServer).toHaveBeenCalledWith("mcp-sentry");
	},
};

/** Navigate to form and back without saving. */
export const BackToList: Story = {
	args: {
		serversData: [
			createServerConfig({
				id: "mcp-sentry",
				display_name: "Sentry",
				slug: "sentry",
			}),
		],
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			await body.findByRole("button", { name: /^Add server$/i }),
		);

		// Click Back.
		await userEvent.click(await body.findByText("Back"));

		// Should be back on the list.
		await expect(
			await body.findByRole("button", { name: /Sentry/ }),
		).toBeInTheDocument();

		expect(args.onCreateServer).not.toHaveBeenCalled();
	},
};

/** Create a server with tool allow/deny lists. */
export const CreateServerWithToolGovernance: Story = {
	args: {
		serversData: [],
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			await body.findByRole("button", { name: /Add your first server/i }),
		);

		await userEvent.type(
			await body.findByLabelText(/Display Name/i),
			"Restricted Server",
		);
		await userEvent.type(
			body.getByLabelText(/Server URL/i),
			"https://mcp.example.com/v1",
		);

		await userEvent.click(body.getByRole("button", { name: /Behavior/i }));
		await userEvent.type(
			body.getByLabelText(/Tool Allow List/i),
			"search, read_file",
		);
		await userEvent.type(
			body.getByLabelText(/Tool Deny List/i),
			"delete_file, execute",
		);

		await userEvent.click(body.getByRole("button", { name: /Create server/i }));

		await waitFor(() => {
			expect(args.onCreateServer).toHaveBeenCalledTimes(1);
		});
		expect(args.onCreateServer).toHaveBeenCalledWith(
			expect.objectContaining({
				tool_allow_list: ["search", "read_file"],
				tool_deny_list: ["delete_file", "execute"],
			}),
		);
	},
};

/** Selecting Custom Headers auth type and adding a header via the key-value editor. */
export const CustomHeadersAuthType: Story = {
	args: {
		serversData: [],
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			await body.findByRole("button", { name: /Add your first server/i }),
		);

		await userEvent.type(
			await body.findByLabelText(/Display Name/i),
			"Custom API",
		);
		await userEvent.type(
			body.getByLabelText(/Server URL/i),
			"https://mcp.example.com/v1",
		);

		await userEvent.click(
			await body.findByRole("button", { name: /Authentication/i }),
		);
		// Select Custom Headers auth type.
		await userEvent.click(body.getByLabelText(/Authentication/i));
		await userEvent.click(
			await body.findByRole("option", { name: /Custom Headers/i }),
		);

		// Add a header.
		await userEvent.click(
			await body.findByRole("button", { name: /Add header/i }),
		);

		await userEvent.type(body.getByLabelText(/Header 1 name/i), "X-Api-Token");
		await userEvent.type(
			body.getByLabelText(/Header 1 value/i),
			"secret-token-123",
		);

		// Submit.
		await userEvent.click(body.getByRole("button", { name: /Create server/i }));

		await waitFor(() => {
			expect(args.onCreateServer).toHaveBeenCalledTimes(1);
		});
		expect(args.onCreateServer).toHaveBeenCalledWith(
			expect.objectContaining({
				auth_type: "custom_headers",
				custom_headers: { "X-Api-Token": "secret-token-123" },
			}),
		);
	},
};
