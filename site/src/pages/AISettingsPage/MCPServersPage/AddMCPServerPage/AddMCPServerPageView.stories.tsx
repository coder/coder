import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { MockMCPServerConfig } from "#/testHelpers/chatEntities";
import { MockGithubExternalProvider } from "#/testHelpers/entities";
import AddMCPServerPageView from "./AddMCPServerPageView";

const meta: Meta<typeof AddMCPServerPageView> = {
	title: "pages/AISettingsPage/MCPServersPage/AddMCPServerPageView",
	component: AddMCPServerPageView,
	args: {
		isSaving: false,
		externalAuthProviders: [MockGithubExternalProvider],
		isLoadingExternalAuthProviders: false,
		accessURL: "https://coder.example.com",
		onCreateServer: fn(
			async (req: TypesGen.CreateMCPServerConfigRequest) =>
				({ ...MockMCPServerConfig, ...req }) as TypesGen.MCPServerConfig,
		),
		onCancel: fn(),
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers/add" },
			routing: { path: "/ai/settings/mcp-servers/add" },
		}),
	},
};

export default meta;
type Story = StoryObj<typeof AddMCPServerPageView>;

export const Default: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const addButton = canvas.getByRole("button", { name: "Add server" });

		await expect(addButton).toBeDisabled();
		await userEvent.type(canvas.getByLabelText(/display name/i), "GitHub");
		await expect(canvas.getByLabelText(/^slug/i)).toHaveValue("github");
		await expect(canvas.getByLabelText("Gateway URL")).toHaveValue(
			"https://coder.example.com/api/v2/ai-gateway/mcp/github",
		);
		await userEvent.type(
			canvas.getByLabelText(/server url/i),
			"https://api.githubcopilot.com/mcp/",
		);
		await expect(addButton).toBeEnabled();

		await userEvent.click(
			canvas.getByRole("button", { name: /authentication/i }),
		);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			canvas.getByRole("combobox", { name: /authentication method/i }),
		);
		await userEvent.click(body.getByRole("option", { name: "OAuth2" }));
		await expect(canvas.getByLabelText(/client id/i)).toBeInTheDocument();

		await userEvent.click(addButton);
		await waitFor(() => {
			expect(args.onCreateServer).toHaveBeenCalledWith(
				expect.objectContaining({
					display_name: "GitHub",
					slug: "github",
					url: "https://api.githubcopilot.com/mcp/",
					auth_type: "oauth2",
				}),
			);
		});
	},
};

export const EscalateToolRule: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.type(canvas.getByLabelText(/display name/i), "GitHub");
		await userEvent.type(
			canvas.getByLabelText(/server url/i),
			"https://api.githubcopilot.com/mcp/",
		);

		await userEvent.click(canvas.getByRole("button", { name: /tool rules/i }));
		await userEvent.click(canvas.getByRole("button", { name: /add rule/i }));
		await userEvent.type(canvas.getByLabelText(/tool name/i), "create_issue");
		await userEvent.click(canvas.getByRole("combobox", { name: /action/i }));
		await userEvent.click(
			body.getByRole("option", { name: /escalate \(require approval\)/i }),
		);

		await userEvent.click(canvas.getByRole("button", { name: "Add server" }));
		await waitFor(() => {
			expect(args.onCreateServer).toHaveBeenCalledWith(
				expect.objectContaining({
					tool_rules: [
						// Escalate mirrors as enabled=false so legacy readers
						// fail closed.
						{ tool: "create_issue", action: "escalate", enabled: false },
					],
				}),
			);
		});
	},
};

export const ExternalAuthSelected: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.type(canvas.getByLabelText(/display name/i), "GitHub");
		await userEvent.type(
			canvas.getByLabelText(/server url/i),
			"https://api.githubcopilot.com/mcp/",
		);
		await userEvent.click(
			canvas.getByRole("button", { name: /authentication/i }),
		);
		await userEvent.click(
			canvas.getByRole("combobox", { name: /authentication method/i }),
		);
		await userEvent.click(
			body.getByRole("option", { name: "External auth provider" }),
		);
		await userEvent.click(
			canvas.getByRole("combobox", { name: /^external auth provider/i }),
		);
		await userEvent.click(body.getByRole("option", { name: "GitHub" }));

		await userEvent.click(canvas.getByRole("button", { name: "Add server" }));
		await waitFor(() => {
			expect(args.onCreateServer).toHaveBeenCalledWith(
				expect.objectContaining({
					auth_type: "external_auth",
					external_auth_provider_id: "github",
				}),
			);
		});
	},
};

export const ToolRulesEditor: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const addButton = canvas.getByRole("button", { name: "Add server" });

		await userEvent.type(canvas.getByLabelText(/display name/i), "GitHub");
		await userEvent.type(
			canvas.getByLabelText(/server url/i),
			"https://api.githubcopilot.com/mcp/",
		);
		await userEvent.click(canvas.getByRole("button", { name: /tool rules/i }));

		const defaultSelector = canvas.getByRole("combobox", {
			name: "Default tool state",
		});
		await userEvent.click(defaultSelector);
		await userEvent.click(body.getByRole("option", { name: "Disabled" }));
		await expect(defaultSelector).toHaveTextContent("Disabled");

		await userEvent.click(canvas.getByRole("button", { name: "Add rule" }));
		const firstRule = canvas.getByRole("group", { name: "Rule 1" });
		const firstToolName = within(firstRule).getByRole("textbox", {
			name: /^Tool name/,
		});
		const firstAction = within(firstRule).getByRole("combobox", {
			name: "Action",
		});
		await expect(within(firstRule).getByRole("alert")).toHaveTextContent(
			"Enter a tool name.",
		);
		await expect(addButton).toBeDisabled();
		await expect(firstAction).toHaveTextContent("Enabled");
		await userEvent.type(firstToolName, "search");
		await userEvent.click(firstAction);
		await userEvent.click(body.getByRole("option", { name: "Disabled" }));
		await expect(firstAction).toHaveTextContent("Disabled");

		await userEvent.click(canvas.getByRole("button", { name: "Add rule" }));
		const secondRule = canvas.getByRole("group", { name: "Rule 2" });
		await userEvent.type(
			within(secondRule).getByRole("textbox", { name: /^Tool name/ }),
			"search",
		);
		await expect(within(secondRule).getByRole("alert")).toHaveTextContent(
			"Tool names must be unique.",
		);
		await expect(addButton).toBeDisabled();
		expect(args.onCreateServer).not.toHaveBeenCalled();

		await userEvent.click(
			within(secondRule).getByRole("button", { name: "Remove rule 2" }),
		);
		await expect(
			canvas.queryByRole("group", { name: "Rule 2" }),
		).not.toBeInTheDocument();
		await expect(
			within(firstRule).queryByRole("alert"),
		).not.toBeInTheDocument();
		await expect(addButton).toBeEnabled();

		await userEvent.click(addButton);
		await waitFor(() => {
			expect(args.onCreateServer).toHaveBeenCalledWith(
				expect.objectContaining({
					tool_default: "disabled",
					tool_rules: [{ tool: "search", action: "disabled", enabled: false }],
				}),
			);
		});
	},
};

export const ExternalAuthProviderRequired: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.type(canvas.getByLabelText(/display name/i), "GitHub");
		await userEvent.type(
			canvas.getByLabelText(/server url/i),
			"https://api.githubcopilot.com/mcp/",
		);
		await userEvent.click(
			canvas.getByRole("button", { name: /authentication/i }),
		);
		await userEvent.click(
			canvas.getByRole("combobox", { name: /authentication method/i }),
		);
		await userEvent.click(
			body.getByRole("option", { name: "External auth provider" }),
		);

		await expect(canvas.getByRole("alert")).toHaveTextContent(
			"Select an external auth provider.",
		);
		await expect(
			canvas.getByRole("button", { name: "Add server" }),
		).toBeDisabled();
	},
};

export const NoExternalAuthProviders: Story = {
	args: {
		externalAuthProviders: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			canvas.getByRole("button", { name: /authentication/i }),
		);
		await expect(
			canvas.getByText(/no external auth providers are configured/i),
		).toBeVisible();
		await userEvent.click(
			canvas.getByRole("combobox", { name: /authentication method/i }),
		);
		// Radix marks disabled options with aria-disabled rather than the
		// native disabled attribute.
		await expect(
			body.getByRole("option", { name: "External auth provider" }),
		).toHaveAttribute("aria-disabled", "true");
	},
};

export const ExternalAuthProvidersLoading: Story = {
	args: {
		externalAuthProviders: [],
		isLoadingExternalAuthProviders: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /authentication/i }),
		);
		await expect(
			canvas.getByText("Loading configured external auth providers."),
		).toBeVisible();
	},
};

export const ExternalAuthProvidersError: Story = {
	args: {
		externalAuthProviders: [],
		externalAuthProvidersError: new Error(
			"Configured external auth providers are unavailable.",
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /authentication/i }),
		);
		await expect(
			canvas.getByText("Configured external auth providers are unavailable."),
		).toBeVisible();
	},
};
