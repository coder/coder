import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
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
	created_at: overrides.created_at ?? now,
	updated_at: overrides.updated_at ?? now,
	auth_connected: overrides.auth_connected ?? false,
});

/**
 * Set up spies for MCP server config API methods. The mutable
 * `state` object lets mutation spies update what queries return
 * on refetch, mimicking a real server round-trip.
 */
const setupMCPSpies = (state: { servers: TypesGen.MCPServerConfig[] }) => {
	spyOn(API.experimental, "getMCPServerConfigs").mockImplementation(
		async () => {
			return state.servers;
		},
	);

	spyOn(API.experimental, "createMCPServerConfig").mockImplementation(
		async (req) => {
			const created = createServerConfig({
				id: `mcp-${Date.now()}`,
				display_name: req.display_name,
				slug: req.slug,
				description: req.description,
				icon_url: req.icon_url,
				transport: req.transport,
				url: req.url,
				auth_type: req.auth_type,
				availability: req.availability,
				enabled: req.enabled,
				has_oauth2_secret: (req.oauth2_client_secret ?? "").length > 0,
				has_api_key: (req.api_key_value ?? "").length > 0,
				has_custom_headers:
					req.custom_headers != null &&
					Object.keys(req.custom_headers).length > 0,
				tool_allow_list: req.tool_allow_list ?? [],
				tool_deny_list: req.tool_deny_list ?? [],
			});
			state.servers = [...state.servers, created];
			return created;
		},
	);

	spyOn(API.experimental, "updateMCPServerConfig").mockImplementation(
		async (id, req) => {
			const idx = state.servers.findIndex((s) => s.id === id);
			if (idx < 0) {
				throw new Error("MCP server config not found.");
			}
			const current = state.servers[idx];
			const updated: TypesGen.MCPServerConfig = {
				...current,
				display_name: req.display_name ?? current.display_name,
				slug: req.slug ?? current.slug,
				description: req.description ?? current.description,
				url: req.url ?? current.url,
				transport: req.transport ?? current.transport,
				auth_type: req.auth_type ?? current.auth_type,
				availability: req.availability ?? current.availability,
				enabled: req.enabled ?? current.enabled,
				updated_at: now,
			};
			state.servers = state.servers.map((s, i) => (i === idx ? updated : s));
			return updated;
		},
	);

	spyOn(API.experimental, "deleteMCPServerConfig").mockImplementation(
		async (id) => {
			state.servers = state.servers.filter((s) => s.id !== id);
		},
	);
};

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
};

export default meta;
type Story = StoryObj<typeof MCPServerAdminPanel>;

// ── Stories ────────────────────────────────────────────────────

/** Empty state with no servers configured. */
export const EmptyState: Story = {
	beforeEach: () => {
		setupMCPSpies({ servers: [] });
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await expect(
			await body.findByText(/No MCP servers configured yet/i),
		).toBeInTheDocument();
		await expect(
			body.getByRole("button", { name: /Add Server/i }),
		).toBeInTheDocument();
	},
};

/** List view with multiple servers showing status indicators. */
export const ServerList: Story = {
	beforeEach: () => {
		setupMCPSpies({
			servers: [
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
		});
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		// All three servers should be visible.
		await expect(
			await body.findByRole("button", { name: /Sentry/ }),
		).toBeInTheDocument();
		expect(body.getByRole("button", { name: /Linear/ })).toBeInTheDocument();
		expect(body.getByRole("button", { name: /GitHub/ })).toBeInTheDocument();
	},
};

/** Navigate to the create form and fill it out. */
export const CreateServer: Story = {
	beforeEach: () => {
		setupMCPSpies({ servers: [] });
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Click Add Server.
		await userEvent.click(
			await body.findByRole("button", { name: /Add Server/i }),
		);

		// Fill in the display name via the inline header input.
		const nameInput = await body.findByLabelText(/Display Name/i);
		await userEvent.type(nameInput, "Sentry");

		// Slug should auto-populate.
		await expect(body.getByLabelText(/^Slug/i)).toHaveValue("sentry");

		await userEvent.type(
			body.getByLabelText(/Server URL/i),
			"https://mcp.sentry.io/sse",
		);

		// Submit.
		await userEvent.click(body.getByRole("button", { name: /Create server/i }));

		await waitFor(() => {
			expect(API.experimental.createMCPServerConfig).toHaveBeenCalledTimes(1);
		});
		expect(API.experimental.createMCPServerConfig).toHaveBeenCalledWith(
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
	beforeEach: () => {
		setupMCPSpies({ servers: [] });
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			await body.findByRole("button", { name: /Add Server/i }),
		);

		await userEvent.type(await body.findByLabelText(/Display Name/i), "GitHub");
		await userEvent.type(
			body.getByLabelText(/Server URL/i),
			"https://api.githubcopilot.com/mcp/",
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
			expect(API.experimental.createMCPServerConfig).toHaveBeenCalledTimes(1);
		});
		expect(API.experimental.createMCPServerConfig).toHaveBeenCalledWith(
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
	beforeEach: () => {
		setupMCPSpies({ servers: [] });
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			await body.findByRole("button", { name: /Add Server/i }),
		);

		await userEvent.type(await body.findByLabelText(/Display Name/i), "Linear");
		await userEvent.type(
			body.getByLabelText(/Server URL/i),
			"https://mcp.linear.app/v1",
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
			expect(API.experimental.createMCPServerConfig).toHaveBeenCalledTimes(1);
		});
		expect(API.experimental.createMCPServerConfig).toHaveBeenCalledWith(
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
	beforeEach: () => {
		setupMCPSpies({
			servers: [
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
		});
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Click the server row.
		await userEvent.click(await body.findByRole("button", { name: /Sentry/ }));

		// The inline name input should be pre-populated.
		const nameInput = await body.findByLabelText(/Display Name/i);
		expect(nameInput).toHaveValue("Sentry");
		expect(body.getByLabelText(/^Slug/i)).toHaveValue("sentry");
		expect(body.getByLabelText(/Server URL/i)).toHaveValue(
			"https://mcp.sentry.io/sse",
		);

		// Update the description.
		const descField = body.getByLabelText(/Description/i);
		await userEvent.clear(descField);
		await userEvent.type(descField, "Sentry error tracking integration");

		await userEvent.click(body.getByRole("button", { name: /Save changes/i }));

		await waitFor(() => {
			expect(API.experimental.updateMCPServerConfig).toHaveBeenCalledTimes(1);
		});
		expect(API.experimental.updateMCPServerConfig).toHaveBeenCalledWith(
			"mcp-sentry",
			expect.objectContaining({
				description: "Sentry error tracking integration",
			}),
		);
	},
};

/** Edit a server that has OAuth2 — secret field should show placeholder. */
export const EditServerWithOAuth2Secret: Story = {
	beforeEach: () => {
		setupMCPSpies({
			servers: [
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
		});
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(await body.findByRole("button", { name: /GitHub/ }));

		// The OAuth2 fields should be visible.
		const secretField = await body.findByLabelText(/Client Secret/i);
		expect(secretField).toHaveValue("••••••••••••••••");
		expect(body.getByLabelText(/Client ID/i)).toHaveValue("gh-client-id");
	},
};

/** Edit a server that has custom headers configured. */
export const EditServerWithCustomHeaders: Story = {
	beforeEach: () => {
		setupMCPSpies({
			servers: [
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
		});
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			await body.findByRole("button", { name: /Custom API/ }),
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
			expect(API.experimental.updateMCPServerConfig).toHaveBeenCalledTimes(1);
		});
		expect(API.experimental.updateMCPServerConfig).toHaveBeenCalledWith(
			"mcp-custom",
			expect.objectContaining({
				custom_headers: { Authorization: "Bearer tok_abc" },
			}),
		);
	},
};

/** Delete a server with confirmation step. */
export const DeleteServerConfirmation: Story = {
	beforeEach: () => {
		setupMCPSpies({
			servers: [
				createServerConfig({
					id: "mcp-sentry",
					display_name: "Sentry",
					slug: "sentry",
				}),
			],
		});
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(await body.findByRole("button", { name: /Sentry/ }));

		// Click Delete.
		await userEvent.click(await body.findByRole("button", { name: "Delete" }));

		// Confirmation should appear.
		await expect(
			await body.findByText(/Are you sure\? This action is irreversible/i),
		).toBeInTheDocument();
	},
};

/** Cancel delete returns to normal form footer. */
export const DeleteServerCancelled: Story = {
	beforeEach: () => {
		setupMCPSpies({
			servers: [
				createServerConfig({
					id: "mcp-sentry",
					display_name: "Sentry",
					slug: "sentry",
				}),
			],
		});
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(await body.findByRole("button", { name: /Sentry/ }));
		await userEvent.click(await body.findByRole("button", { name: "Delete" }));
		await body.findByText(/Are you sure/i);
		await userEvent.click(body.getByRole("button", { name: "Cancel" }));

		// Normal footer should be restored.
		await expect(
			await body.findByRole("button", { name: "Delete" }),
		).toBeInTheDocument();
		expect(
			body.getByRole("button", { name: /Save changes/i }),
		).toBeInTheDocument();
	},
};

/** Confirm delete calls the API. */
export const DeleteServerConfirmed: Story = {
	beforeEach: () => {
		setupMCPSpies({
			servers: [
				createServerConfig({
					id: "mcp-sentry",
					display_name: "Sentry",
					slug: "sentry",
				}),
			],
		});
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(await body.findByRole("button", { name: /Sentry/ }));
		await userEvent.click(await body.findByRole("button", { name: "Delete" }));
		await body.findByText(/Are you sure/i);
		await userEvent.click(body.getByRole("button", { name: /Delete server/i }));

		await waitFor(() => {
			expect(API.experimental.deleteMCPServerConfig).toHaveBeenCalledTimes(1);
		});
		expect(API.experimental.deleteMCPServerConfig).toHaveBeenCalledWith(
			"mcp-sentry",
		);
	},
};

/** Navigate to form and back without saving. */
export const BackToList: Story = {
	beforeEach: () => {
		setupMCPSpies({
			servers: [
				createServerConfig({
					id: "mcp-sentry",
					display_name: "Sentry",
					slug: "sentry",
				}),
			],
		});
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			await body.findByRole("button", { name: /Add Server/i }),
		);

		// Click Back.
		await userEvent.click(await body.findByText("Back"));

		// Should be back on the list.
		await expect(
			await body.findByRole("button", { name: /Sentry/ }),
		).toBeInTheDocument();

		expect(API.experimental.createMCPServerConfig).not.toHaveBeenCalled();
	},
};

/** Create a server with tool allow/deny lists. */
export const CreateServerWithToolGovernance: Story = {
	beforeEach: () => {
		setupMCPSpies({ servers: [] });
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			await body.findByRole("button", { name: /Add Server/i }),
		);

		await userEvent.type(
			await body.findByLabelText(/Display Name/i),
			"Restricted Server",
		);
		await userEvent.type(
			body.getByLabelText(/Server URL/i),
			"https://mcp.example.com/v1",
		);

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
			expect(API.experimental.createMCPServerConfig).toHaveBeenCalledTimes(1);
		});
		expect(API.experimental.createMCPServerConfig).toHaveBeenCalledWith(
			expect.objectContaining({
				tool_allow_list: ["search", "read_file"],
				tool_deny_list: ["delete_file", "execute"],
			}),
		);
	},
};

/** Selecting Custom Headers auth type and adding a header via the key-value editor. */
export const CustomHeadersAuthType: Story = {
	beforeEach: () => {
		setupMCPSpies({ servers: [] });
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			await body.findByRole("button", { name: /Add Server/i }),
		);

		await userEvent.type(
			await body.findByLabelText(/Display Name/i),
			"Custom API",
		);
		await userEvent.type(
			body.getByLabelText(/Server URL/i),
			"https://mcp.example.com/v1",
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
			expect(API.experimental.createMCPServerConfig).toHaveBeenCalledTimes(1);
		});
		expect(API.experimental.createMCPServerConfig).toHaveBeenCalledWith(
			expect.objectContaining({
				auth_type: "custom_headers",
				custom_headers: { "X-Api-Token": "secret-token-123" },
			}),
		);
	},
};
