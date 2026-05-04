import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	expect,
	fn,
	screen,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { ConfirmDialog } from "#/components/Dialogs/ConfirmDialog/ConfirmDialog";
import {
	MockDefaultOrganization,
	MockOrganization2,
	MockWorkspace,
} from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { AgentCreateForm } from "./AgentCreateForm";

// Query key used by permittedOrganizations() in the form.
const permittedOrgsKey = [
	"organizations",
	"permitted",
	{ object: { resource_type: "chat" }, action: "create" },
];

const modelConfigID = "model-config-1";
const claudeModelConfigID = "model-config-claude";

const modelOptions = [
	{
		id: modelConfigID,
		provider: "openai",
		model: "gpt-4o",
		displayName: "GPT-4o",
	},
	{
		id: claudeModelConfigID,
		provider: "anthropic",
		model: "claude-sonnet-4",
		displayName: "Claude Sonnet 4",
	},
] as const;

const buildModelConfig = (
	overrides: Partial<TypesGen.ChatModelConfig> = {},
): TypesGen.ChatModelConfig => ({
	id: modelConfigID,
	provider: "openai",
	model: "gpt-4o",
	display_name: "GPT-4o",
	enabled: true,
	is_default: false,
	context_limit: 200_000,
	compression_threshold: 70,
	created_at: "2026-02-18T00:00:00.000Z",
	updated_at: "2026-02-18T00:00:00.000Z",
	...overrides,
});

const defaultModelConfigs: TypesGen.ChatModelConfig[] = [
	buildModelConfig({ is_default: true }),
	buildModelConfig({
		id: claudeModelConfigID,
		provider: "anthropic",
		model: "claude-sonnet-4",
		display_name: "Claude Sonnet 4",
		context_limit: 200_000,
	}),
];

const buildRootPersonalModelOverride = (
	overrides: Partial<TypesGen.ChatPersonalModelOverride> = {},
): TypesGen.ChatPersonalModelOverride => ({
	context: "root",
	mode: "chat_default",
	model_config_id: "",
	is_set: true,
	is_malformed: false,
	...overrides,
});

const mock403Error = Object.assign(
	new Error("Request failed with status code 403"),
	{
		isAxiosError: true,
		response: {
			status: 403,
			statusText: "Forbidden",
			data: {
				message: "Forbidden.",
				detail: "Insufficient permissions to use Coder Agents.",
			},
			headers: {},
			config: {},
		},
		config: {},
		toJSON: () => ({}),
	},
);

const meta: Meta<typeof AgentCreateForm> = {
	title: "pages/AgentsPage/AgentCreateForm",
	component: AgentCreateForm,
	decorators: [withDashboardProvider],
	args: {
		onCreateChat: fn(),
		isCreating: false,
		createError: undefined,
		canCreateChat: true,
		modelCatalog: null,
		modelOptions: [...modelOptions],
		isModelCatalogLoading: false,
		modelConfigs: [],
		isModelConfigsLoading: false,
		workspaceCount: 0,
		workspaceOptions: [],
		workspacesError: undefined,
		isWorkspacesLoading: false,
	},
	beforeEach: () => {
		localStorage.clear();
	},
};

export default meta;
type Story = StoryObj<typeof AgentCreateForm>;

const defaultArgs = meta.args;

const mockPermittedOrganizations = (permissions: Record<string, boolean>) => {
	spyOn(API, "getOrganizations").mockResolvedValue([
		MockDefaultOrganization,
		MockOrganization2,
	]);
	spyOn(API, "checkAuthorization").mockResolvedValue(permissions);
};

export const Default: Story = {};

const submitMessage = async (canvasElement: HTMLElement, message: string) => {
	const canvas = within(canvasElement);
	const input = canvas.getByTestId("chat-message-input");
	await userEvent.click(input);
	await userEvent.keyboard(message);
	await userEvent.click(canvas.getByRole("button", { name: "Send" }));
};

const getCreateOptions = (onCreateChat: unknown): CreateChatSubmission => {
	const mock = onCreateChat as ReturnType<typeof fn>;
	const options = mock.mock.calls[0]?.[0] as CreateChatSubmission | undefined;
	if (!options) {
		throw new Error("Expected onCreateChat to receive options.");
	}
	return options;
};

type CreateChatSubmission = {
	model?: string;
};

export const RootPersonalModelOverrideModelSelected: Story = {
	args: {
		...defaultArgs,
		onCreateChat: fn().mockResolvedValue(undefined),
		modelConfigs: defaultModelConfigs,
		rootPersonalModelOverride: buildRootPersonalModelOverride({
			mode: "model",
			model_config_id: claudeModelConfigID,
		}),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("combobox", { name: "Claude Sonnet 4" }),
		).toBeInTheDocument();
		await submitMessage(canvasElement, "create with saved root model");
		await waitFor(() => {
			expect(args.onCreateChat).toHaveBeenCalled();
		});
		expect(getCreateOptions(args.onCreateChat).model).toBe(claudeModelConfigID);
	},
};

export const RootChatDefaultSubmitsDisplayedModel: Story = {
	args: {
		...defaultArgs,
		onCreateChat: fn().mockResolvedValue(undefined),
		modelConfigs: defaultModelConfigs,
		rootPersonalModelOverride: buildRootPersonalModelOverride({
			mode: "chat_default",
			model_config_id: "",
		}),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("combobox", { name: "GPT-4o" }),
		).toBeInTheDocument();
		await submitMessage(canvasElement, "create with chat default");
		await waitFor(() => {
			expect(args.onCreateChat).toHaveBeenCalled();
		});
		expect(getCreateOptions(args.onCreateChat).model).toBe(modelConfigID);
	},
};

export const RootOverrideMissingFromCatalog: Story = {
	args: {
		...defaultArgs,
		onCreateChat: fn().mockResolvedValue(undefined),
		modelConfigs: defaultModelConfigs,
		rootPersonalModelOverride: buildRootPersonalModelOverride({
			mode: "model",
			model_config_id: "model-does-not-exist",
			is_set: true,
			is_malformed: false,
		}),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("combobox", { name: "GPT-4o" }),
		).toBeInTheDocument();
		await submitMessage(canvasElement, "create with missing root model");
		await waitFor(() => {
			expect(args.onCreateChat).toHaveBeenCalled();
		});
		expect(getCreateOptions(args.onCreateChat).model).toBe(modelConfigID);
	},
};

export const MalformedRootOverrideUsesDefaultModel: Story = {
	args: {
		...defaultArgs,
		onCreateChat: fn().mockResolvedValue(undefined),
		modelConfigs: defaultModelConfigs,
		rootPersonalModelOverride: buildRootPersonalModelOverride({
			mode: "model",
			model_config_id: claudeModelConfigID,
			is_malformed: true,
		}),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("combobox", { name: "GPT-4o" }),
		).toBeInTheDocument();
		expect(
			canvas.queryByRole("combobox", { name: "Claude Sonnet 4" }),
		).not.toBeInTheDocument();
		await submitMessage(canvasElement, "create with malformed root model");
		await waitFor(() => {
			expect(args.onCreateChat).toHaveBeenCalled();
		});
		expect(getCreateOptions(args.onCreateChat).model).toBe(modelConfigID);
	},
};

export const LastUsedModelFallbackWithoutRootOverride: Story = {
	args: {
		...defaultArgs,
		onCreateChat: fn().mockResolvedValue(undefined),
		modelConfigs: defaultModelConfigs,
	},
	beforeEach: () => {
		localStorage.clear();
		localStorage.setItem("agents.last-model-config-id", claudeModelConfigID);
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("combobox", { name: "Claude Sonnet 4" }),
		).toBeInTheDocument();
		await submitMessage(canvasElement, "create with last used model");
		await waitFor(() => {
			expect(args.onCreateChat).toHaveBeenCalled();
		});
		expect(getCreateOptions(args.onCreateChat).model).toBe(claudeModelConfigID);
	},
};

export const ManualSelectionOverridesRootChatDefault: Story = {
	args: {
		...defaultArgs,
		onCreateChat: fn().mockResolvedValue(undefined),
		modelConfigs: defaultModelConfigs,
		rootPersonalModelOverride: buildRootPersonalModelOverride({
			mode: "chat_default",
			model_config_id: "",
		}),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("combobox", { name: "GPT-4o" }));
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("option", { name: /Claude Sonnet 4/i }),
		);
		await submitMessage(canvasElement, "create with manual model");
		await waitFor(() => {
			expect(args.onCreateChat).toHaveBeenCalled();
		});
		expect(getCreateOptions(args.onCreateChat).model).toBe(claudeModelConfigID);
	},
};

const mockWorkspaces = [
	{
		...MockWorkspace,
		id: "ws-1",
		name: "my-project",
		owner_name: "johndoe",
		owner_id: "user-1",
	},
	{
		...MockWorkspace,
		id: "ws-2",
		name: "my-project",
		owner_name: "janedoe",
		owner_id: "user-2",
	},
	{
		...MockWorkspace,
		id: "ws-3",
		name: "backend-api",
		owner_name: "johndoe",
		owner_id: "user-1",
	},
];

export const WithWorkspaces: Story = {
	args: {
		workspaceOptions: mockWorkspaces,
		workspaceCount: mockWorkspaces.length,
	},
	beforeEach: () => {
		localStorage.clear();
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		// Open the "+" menu first, then click the workspace trigger inside it.
		await userEvent.click(canvas.getByRole("button", { name: "More options" }));
		await waitFor(() => {
			const trigger = body.getByText("Attach workspace").closest("button")!;
			expect(trigger).toBeEnabled();
		});
		await userEvent.click(
			body.getByText("Attach workspace").closest("button")!,
		);
		// Wait for the workspace combobox dropdown to appear so
		// Chromatic captures it.
		await body.findByPlaceholderText("Search workspaces...");
	},
};

export const SearchWorkspaces: Story = {
	args: {
		workspaceOptions: mockWorkspaces,
		workspaceCount: mockWorkspaces.length,
	},
	beforeEach: () => {
		localStorage.clear();
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		// Open the "+" menu first, then click the workspace trigger inside it.
		await userEvent.click(canvas.getByRole("button", { name: "More options" }));
		await waitFor(() => {
			const trigger = body.getByText("Attach workspace").closest("button")!;
			expect(trigger).toBeEnabled();
		});
		await userEvent.click(
			body.getByText("Attach workspace").closest("button")!,
		);

		// Type in the search input to filter workspaces.
		const searchInput = body.getByPlaceholderText("Search workspaces...");
		await userEvent.type(searchInput, "backend");

		// Only the matching workspace should remain visible.
		await waitFor(() => {
			const options = body.getAllByRole("option");
			// "Auto-create Workspace" is filtered out, only
			// "johndoe/backend-api" matches.
			expect(options).toHaveLength(1);
			expect(options[0]).toHaveTextContent("backend-api");
		});
	},
};

export const SelectWorkspaceViaSearch: Story = {
	args: {
		workspaceOptions: mockWorkspaces,
		workspaceCount: mockWorkspaces.length,
	},
	beforeEach: () => {
		localStorage.clear();
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		// Open the "+" menu first, then click the workspace trigger inside it.
		await userEvent.click(canvas.getByRole("button", { name: "More options" }));
		await waitFor(() => {
			const trigger = body.getByText("Attach workspace").closest("button")!;
			expect(trigger).toBeEnabled();
		});
		await userEvent.click(
			body.getByText("Attach workspace").closest("button")!,
		);

		// Search for "backend" and select the result.
		const searchInput = body.getByPlaceholderText("Search workspaces...");
		await userEvent.type(searchInput, "backend");

		await waitFor(() => {
			expect(body.getAllByRole("option")).toHaveLength(1);
		});

		await userEvent.click(body.getByRole("option", { name: /backend-api/ }));

		// Re-open the "+" menu to verify the selected workspace label.
		await userEvent.click(canvas.getByRole("button", { name: "More options" }));
		await waitFor(() => {
			expect(body.getByText("backend-api")).toBeInTheDocument();
		});
	},
};

export const LoadingModelCatalog: Story = {
	args: {
		...defaultArgs,
		modelCatalog: null,
		modelOptions: [],
		isModelCatalogLoading: true,
		isModelConfigsLoading: true,
	},
};

export const LoadingPersonalModelOverrides: Story = {
	args: {
		...defaultArgs,
		isPersonalModelOverridesLoading: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("textbox")).toHaveAttribute(
			"aria-disabled",
			"true",
		);
	},
};

export const NoModelsConfigured: Story = {
	args: {
		...defaultArgs,
		modelCatalog: { providers: [] },
		modelOptions: [],
		isModelCatalogLoading: false,
		isModelConfigsLoading: false,
	},
};

export const PreservesAttachmentsOnFailedSend: Story = {
	args: {
		...defaultArgs,
		onCreateChat: fn().mockRejectedValue(new Error("server error")),
	},
	beforeEach: () => {
		localStorage.clear();
		// Pre-persist an uploaded attachment so it is restored on mount.
		localStorage.setItem(
			"agents.persisted-attachments",
			JSON.stringify([
				{
					fileId: "persisted-file-1",
					fileName: "photo.png",
					fileType: "image/png",
					lastModified: 1000,
					organizationId: "my-organization-id",
				},
			]),
		);
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		// The restored attachment should appear on mount.
		await waitFor(() => {
			expect(canvas.getByLabelText("Remove photo.png")).toBeInTheDocument();
		});

		// Type a message and submit.
		const input = canvas.getByTestId("chat-message-input");
		await userEvent.click(input);
		await userEvent.keyboard("test message");
		await userEvent.click(canvas.getByRole("button", { name: "Send" }));

		// Wait for onCreateChat to have been called (and rejected).
		await waitFor(() => {
			expect(args.onCreateChat).toHaveBeenCalled();
		});

		// The attachment must still be visible after the failed send.
		await waitFor(() => {
			expect(canvas.getByLabelText("Remove photo.png")).toBeInTheDocument();
		});

		// localStorage must still have the persisted attachment.
		const stored = localStorage.getItem("agents.persisted-attachments");
		expect(stored).not.toBeNull();
		const parsed = JSON.parse(stored!);
		expect(parsed).toHaveLength(1);
		expect(parsed[0].fileId).toBe("persisted-file-1");
	},
};

export const UsageLimitExceeded: Story = {
	args: {
		...defaultArgs,
		createError: Object.assign(
			new Error("Request failed with status code 409"),
			{
				isAxiosError: true,
				response: {
					status: 409,
					statusText: "Conflict",
					data: {
						message: "Chat usage limit exceeded.",
						spent_micros: 900_000,
						limit_micros: 500_000,
						resets_at: "2026-03-16T00:00:00Z",
					},
					headers: {},
					config: {},
				},
				config: {},
				toJSON: () => ({}),
			},
		),
	},
};

export const ForbiddenErrorWithRole: Story = {
	args: {
		...defaultArgs,
		canCreateChat: true,
		createError: mock403Error,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// The friendly "role required" alert must NOT appear because the
		// user has the agents-access role.
		await expect(
			canvas.queryByText("Permission required"),
		).not.toBeInTheDocument();
		// The generic ErrorAlert should surface the real backend message.
		await expect(canvas.getByText("Forbidden.")).toBeInTheDocument();
		// The textbox should remain enabled since the user has the role.
		const textbox = canvas.getByRole("textbox");
		await expect(textbox).not.toHaveAttribute("aria-disabled", "true");
	},
};

export const WithOrganizationPicker: Story = {
	parameters: {
		showOrganizations: true,
		organizations: [MockDefaultOrganization, MockOrganization2],
		queries: [
			{
				key: permittedOrgsKey,
				data: [MockDefaultOrganization, MockOrganization2],
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Verify the org picker rendered (component didn't crash).
		await waitFor(() => {
			expect(canvas.getByTestId("compact-org-selector")).toBeInTheDocument();
		});
		// Type into the chat input to trigger re-renders. If the
		// permittedOrgs fallback is referentially unstable, this
		// causes a render cascade that hits React's update limit.
		const input = canvas.getByTestId("chat-message-input");
		await userEvent.click(input);
		await userEvent.keyboard("hello world");
		// The org picker should still be present after typing.
		expect(canvas.getByTestId("compact-org-selector")).toBeInTheDocument();
	},
};

/**
 * Standalone story for the org-change confirmation dialog. Renders
 * the ConfirmDialog directly in its open state, following the same
 * pattern as DeleteConfirmationDialog in AgentsPageView.stories.
 */
export const OrgChangeConfirmation: Story = {
	render: () => (
		<ConfirmDialog
			open={true}
			title="Change organization?"
			description="Changing organization will remove your current attachments."
			type="info"
			hideCancel={false}
			confirmText="Continue"
			onConfirm={fn()}
			onClose={fn()}
		/>
	),
	play: async () => {
		const dialog = await screen.findByRole("dialog");
		await expect(dialog).toBeInTheDocument();
		await expect(
			within(dialog).getByText("Change organization?"),
		).toBeInTheDocument();
		await expect(
			within(dialog).getByText(
				"Changing organization will remove your current attachments.",
			),
		).toBeInTheDocument();
		await expect(
			within(dialog).getByRole("button", { name: /continue/i }),
		).toBeInTheDocument();
		await expect(
			within(dialog).getByRole("button", { name: /cancel/i }),
		).toBeInTheDocument();
	},
};

export const ForbiddenNoAgentsRole: Story = {
	args: {
		...defaultArgs,
		canCreateChat: false,
		createError: mock403Error,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Permission required")).toBeInTheDocument();
		await expect(
			canvas.getByRole("link", { name: /View Docs/ }),
		).toBeInTheDocument();
		await expect(
			canvas.queryByRole("heading", { name: "Forbidden." }),
		).not.toBeInTheDocument();
		// The textarea should be disabled so the user cannot
		// accidentally trigger the generic error.
		const textbox = canvas.getByRole("textbox");
		await expect(textbox).toHaveAttribute("aria-disabled", "true");
	},
};

/**
 * Covers the reconciliation path where the permitted-organizations query
 * resolves after mount with fewer orgs than the dashboard provides.
 */
export const PermittedOrgsResolvesToEmpty: Story = {
	parameters: {
		showOrganizations: true,
		organizations: [MockDefaultOrganization, MockOrganization2],
		// Deliberately do not pre-seed permittedOrgsKey. Let the
		// mocked API calls drive the async permission resolution.
	},
	args: {
		...defaultArgs,
		onCreateChat: fn().mockResolvedValue(undefined),
	},
	beforeEach: () => {
		mockPermittedOrganizations({
			[MockDefaultOrganization.id]: false,
			[MockOrganization2.id]: false,
		});
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		// Wait for the permitted orgs query to resolve. The org picker
		// should disappear since no org is permitted.
		await waitFor(
			() => {
				expect(
					canvas.queryByTestId("compact-org-selector"),
				).not.toBeInTheDocument();
			},
			{ timeout: 3000 },
		);

		// Type a message and submit the form.
		const input = canvas.getByTestId("chat-message-input");
		await userEvent.click(input);
		await userEvent.keyboard("test message");
		await userEvent.click(canvas.getByRole("button", { name: "Send" }));

		// Verify onCreateChat was called with a non-empty organizationId.
		await waitFor(() => {
			expect(args.onCreateChat).toHaveBeenCalled();
		});
		const options = (args.onCreateChat as ReturnType<typeof fn>).mock
			.calls[0]?.[0] as { organizationId: string } | undefined;
		if (!options) {
			throw new Error("Expected onCreateChat to receive options");
		}
		expect(options.organizationId).not.toBe("");
		// It should fall back to the default org from the dashboard.
		expect(options.organizationId).toBe(MockDefaultOrganization.id);
	},
};

export const PermittedOrgsResolvesToSubset: Story = {
	parameters: {
		showOrganizations: true,
		organizations: [MockDefaultOrganization, MockOrganization2],
	},
	args: {
		...defaultArgs,
		onCreateChat: fn().mockResolvedValue(undefined),
	},
	beforeEach: () => {
		mockPermittedOrganizations({
			[MockDefaultOrganization.id]: false,
			[MockOrganization2.id]: true,
		});
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		// Wait for the permitted orgs query to resolve. With only one
		// permitted org, the picker should disappear.
		await waitFor(
			() => {
				expect(
					canvas.queryByTestId("compact-org-selector"),
				).not.toBeInTheDocument();
			},
			{ timeout: 3000 },
		);

		// Type a message and submit.
		const input = canvas.getByTestId("chat-message-input");
		await userEvent.click(input);
		await userEvent.keyboard("test message");
		await userEvent.click(canvas.getByRole("button", { name: "Send" }));

		// Verify onCreateChat was called with the only permitted org.
		await waitFor(() => {
			expect(args.onCreateChat).toHaveBeenCalled();
		});
		const options = (args.onCreateChat as ReturnType<typeof fn>).mock
			.calls[0]?.[0] as { organizationId: string } | undefined;
		if (!options) {
			throw new Error("Expected onCreateChat to receive options");
		}
		expect(options.organizationId).toBe(MockOrganization2.id);
	},
};
