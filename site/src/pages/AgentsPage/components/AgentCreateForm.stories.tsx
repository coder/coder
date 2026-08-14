import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { delay } from "msw";
import { useState } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
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
import { permittedOrganizationsKey } from "#/api/queries/organizations";
import type * as TypesGen from "#/api/typesGenerated";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { MockChatModelConfig } from "#/testHelpers/chatModels";
import {
	MockDefaultOrganization,
	MockOrganization2,
	MockWorkspace,
} from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { persistedAttachmentsStorageKey } from "../hooks/useFileAttachments";
import {
	getReasoningEffortForModel,
	saveReasoningEffortForModel,
} from "../utils/reasoningEffort";
import {
	AgentCreateForm,
	type CreateChatOptions,
	emptyInputStorageKey,
} from "./AgentCreateForm";

const permittedOrgsKey = permittedOrganizationsKey({
	object: { resource_type: "chat", owner_id: "me" },
	action: "create",
});

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
	...MockChatModelConfig,
	id: modelConfigID,
	model: "gpt-4o",
	display_name: "GPT-4o",
	created_at: "2026-02-18T00:00:00.000Z",
	updated_at: "2026-02-18T00:00:00.000Z",
	...overrides,
});

const defaultModelConfigs: TypesGen.ChatModelConfig[] = [
	buildModelConfig({ is_default: true }),
	buildModelConfig({
		id: claudeModelConfigID,
		ai_provider_id: "provider-anthropic",
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
		sendShortcut: "enter",
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

const mockPermittedOrganizations = (
	permissions: Record<string, boolean>,
	delayMs = 0,
) => {
	spyOn(API, "getOrganizations").mockResolvedValue([
		MockDefaultOrganization,
		MockOrganization2,
	]);
	spyOn(API, "checkAuthorization").mockImplementation(async () => {
		if (delayMs > 0) {
			await delay(delayMs);
		}
		return permissions;
	});
};

export const Default: Story = {};

const submitMessage = async (canvasElement: HTMLElement, message: string) => {
	const canvas = within(canvasElement);
	const input = canvas.getByRole("textbox", { name: "Chat message" });
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

type CreateChatSubmission = Pick<
	CreateChatOptions,
	"model" | "reasoningEffort" | "runtime" | "workspaceId"
>;

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

// Model options with reasoning effort bounds configured. GPT-4o uses the
// full global scale; Claude is capped at medium.
const effortModelOptions = [
	{
		...modelOptions[0],
		reasoningEffortDefault: "medium",
		reasoningEfforts: [
			"none",
			"minimal",
			"low",
			"medium",
			"high",
			"xhigh",
			"max",
		],
	},
	{
		...modelOptions[1],
		reasoningEffortDefault: "low",
		reasoningEfforts: ["low", "medium"],
	},
] as const;

export const RemembersReasoningEffortByModel: Story = {
	args: {
		...defaultArgs,
		modelOptions: [...effortModelOptions],
	},
	beforeEach: () => {
		localStorage.clear();
		saveReasoningEffortForModel(modelConfigID, "high");
		saveReasoningEffortForModel(claudeModelConfigID, "medium");
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const modelSelector = canvas.getByRole("combobox", { name: "GPT-4o" });

		await userEvent.click(modelSelector);
		expect(await body.findByRole("slider")).toHaveAttribute(
			"aria-valuenow",
			"4",
		);
		await userEvent.click(
			await body.findByRole("option", { name: /Claude Sonnet 4/i }),
		);

		await userEvent.click(
			canvas.getByRole("combobox", { name: "Claude Sonnet 4" }),
		);
		expect(await body.findByRole("slider")).toHaveAttribute(
			"aria-valuenow",
			"1",
		);
		await userEvent.click(await body.findByRole("option", { name: /GPT-4o/i }));

		await userEvent.click(canvas.getByRole("combobox", { name: "GPT-4o" }));
		const restoredSlider = await body.findByRole("slider");
		expect(restoredSlider).toHaveAttribute("aria-valuenow", "4");
		restoredSlider.focus();
		await userEvent.keyboard("{ArrowRight}");
		await waitFor(() => {
			expect(getReasoningEffortForModel(modelConfigID)).toBe("xhigh");
		});
		await userEvent.keyboard("{Escape}");
	},
};

export const PersistedReasoningEffortOutranksRootOverride: Story = {
	args: {
		...defaultArgs,
		onCreateChat: fn().mockResolvedValue(undefined),
		modelOptions: [...effortModelOptions],
		modelConfigs: defaultModelConfigs,
		rootPersonalModelOverride: buildRootPersonalModelOverride({
			mode: "model",
			model_config_id: modelConfigID,
			reasoning_effort: "high",
		}),
	},
	beforeEach: () => {
		localStorage.clear();
		saveReasoningEffortForModel(modelConfigID, "low");
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		// The persisted per-model value wins over the root override.
		await userEvent.click(canvas.getByRole("combobox", { name: "GPT-4o" }));
		expect(await body.findByRole("slider")).toHaveAttribute(
			"aria-valuenow",
			"2",
		);
		await userEvent.keyboard("{Escape}");

		await submitMessage(canvasElement, "create with persisted effort");
		await waitFor(() => {
			expect(args.onCreateChat).toHaveBeenCalled();
		});
		expect(getCreateOptions(args.onCreateChat).reasoningEffort).toBe("low");
	},
};

export const ManualReselectKeepsRootOverrideEffort: Story = {
	args: {
		...defaultArgs,
		modelOptions: [...effortModelOptions],
		modelConfigs: defaultModelConfigs,
		rootPersonalModelOverride: buildRootPersonalModelOverride({
			mode: "model",
			model_config_id: modelConfigID,
			reasoning_effort: "high",
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		// Re-selecting the override's own model keeps the override effort.
		await userEvent.click(canvas.getByRole("combobox", { name: "GPT-4o" }));
		await userEvent.click(await body.findByRole("option", { name: /GPT-4o/i }));
		await userEvent.click(canvas.getByRole("combobox", { name: "GPT-4o" }));
		expect(await body.findByRole("slider")).toHaveAttribute(
			"aria-valuenow",
			"4",
		);
		await userEvent.keyboard("{Escape}");
	},
};

export const StalePersistedEffortFallsThroughToRootOverride: Story = {
	args: {
		...defaultArgs,
		onCreateChat: fn().mockResolvedValue(undefined),
		modelOptions: [
			{
				...modelOptions[0],
				reasoningEffortDefault: "low",
				reasoningEfforts: ["low", "medium"],
			},
		],
		modelConfigs: defaultModelConfigs,
		rootPersonalModelOverride: buildRootPersonalModelOverride({
			mode: "model",
			model_config_id: modelConfigID,
			reasoning_effort: "medium",
		}),
	},
	beforeEach: () => {
		localStorage.clear();
		saveReasoningEffortForModel(modelConfigID, "max");
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		// The stored "max" is no longer valid for this model, so the
		// root override's "medium" applies instead of the default "low".
		await userEvent.click(canvas.getByRole("combobox", { name: "GPT-4o" }));
		expect(await body.findByRole("slider")).toHaveAttribute(
			"aria-valuenow",
			"1",
		);
		await userEvent.keyboard("{Escape}");

		await submitMessage(canvasElement, "create with stale persisted effort");
		await waitFor(() => {
			expect(args.onCreateChat).toHaveBeenCalled();
		});
		expect(getCreateOptions(args.onCreateChat).reasoningEffort).toBe("medium");
	},
};

export const SubmitsReasoningEffort: Story = {
	// TODO: This story fails when pixel runs its play function. Fix it and remove the exclude.
	parameters: { pixel: { exclude: true } },
	args: {
		...defaultArgs,
		onCreateChat: fn().mockResolvedValue(undefined),
		modelOptions: [...effortModelOptions],
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		// Open the model selector; the effort row shows the model default.
		await userEvent.click(canvas.getByRole("combobox", { name: "GPT-4o" }));
		const slider = await body.findByRole("slider");
		// "medium" is the fourth of seven selectable efforts.
		expect(slider).toHaveAttribute("aria-valuenow", "3");

		// Bump the effort to "high" with the keyboard, then close.
		// The info button precedes the slider in tab order.
		await userEvent.tab();
		expect(
			body.getByRole("button", { name: "About reasoning effort" }),
		).toHaveFocus();
		await userEvent.tab();
		expect(slider).toHaveFocus();
		await userEvent.keyboard("{ArrowRight}");
		await waitFor(() => {
			expect(slider).toHaveAttribute("aria-valuenow", "4");
		});
		await userEvent.keyboard("{Escape}");

		await submitMessage(canvasElement, "create with reasoning effort");
		await waitFor(() => {
			expect(args.onCreateChat).toHaveBeenCalled();
		});
		const options = getCreateOptions(args.onCreateChat);
		expect(options.model).toBe(modelConfigID);
		expect(options.reasoningEffort).toBe("high");
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
		// Wait for the workspace combobox dropdown to appear so snapshot tests
		// capture it.
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
	// TODO: This story fails when pixel runs its play function. Fix it and remove the exclude.
	parameters: { pixel: { exclude: true } },
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
		modelCatalog: { providers: [], unsupported_providers: [] },
		modelOptions: [],
		isModelCatalogLoading: false,
		isModelConfigsLoading: false,
	},
};

export const MissingProviderAndModelSetup: Story = {
	args: {
		...defaultArgs,
		canConfigureAgentSetup: true,
		providerCount: 0,
		modelCount: 0,
		modelCatalog: { providers: [], unsupported_providers: [] },
		modelOptions: [],
		isModelCatalogLoading: false,
		isModelConfigsLoading: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await waitFor(() => {
			expect(
				canvas.getAllByText((_content, element) => {
					return (
						element?.textContent ===
						"To chat with Coder Agents, set up a provider then add a model."
					);
				})[0],
			).toBeVisible();
		});
		expect(canvas.getByRole("link", { name: "provider" })).toHaveAttribute(
			"href",
			"/ai/settings/providers",
		);
		expect(canvas.getByRole("link", { name: "model" })).toHaveAttribute(
			"href",
			"/ai/settings/models",
		);
	},
};

export const AIGatewayDisabled: Story = {
	args: {
		...defaultArgs,
		aiGatewayDisabled: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("textbox")).toHaveAttribute(
			"aria-disabled",
			"true",
		);
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

export const HookDispatchFailed: Story = {
	args: {
		...defaultArgs,
		createError: Object.assign(
			new Error("Request failed with status code 502"),
			{
				isAxiosError: true,
				response: {
					status: 502,
					statusText: "Bad Gateway",
					data: {
						kind: "hook_dispatch_failed",
						message: "Chat lifecycle hook dispatch failed.",
						detail:
							"Lifecycle hook dispatch 00000000-0000-0000-0000-000000000001 failed (http_error).",
					},
					headers: {},
					config: {},
				},
				config: {},
				toJSON: () => ({}),
			},
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Lifecycle hook failed")).toBeVisible();
		await expect(
			canvas.getByText("Chat lifecycle hook dispatch failed."),
		).toBeVisible();
		await expect(
			canvas.getByText(
				"Lifecycle hook dispatch 00000000-0000-0000-0000-000000000001 failed (http_error).",
			),
		).toBeVisible();
		await expect(canvas.queryByText("Stack Trace")).not.toBeInTheDocument();
		await expect(canvas.queryByText("Response data")).not.toBeInTheDocument();
	},
};

export const HookDenied: Story = {
	args: {
		...defaultArgs,
		createError: Object.assign(
			new Error("Request failed with status code 403"),
			{
				isAxiosError: true,
				response: {
					status: 403,
					statusText: "Forbidden",
					data: {
						kind: "hook_denied",
						message: "This prompt is blocked by policy.",
					},
					headers: {},
					config: {},
				},
				config: {},
				toJSON: () => ({}),
			},
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("This prompt is blocked by policy."),
		).toBeVisible();
		await expect(
			canvas.queryByText("Blocked by policy"),
		).not.toBeInTheDocument();
		await expect(
			canvas.queryByText("Go to workspaces"),
		).not.toBeInTheDocument();
		await expect(canvas.queryByText("Stack Trace")).not.toBeInTheDocument();
		await expect(canvas.queryByText("Response data")).not.toBeInTheDocument();
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
				data: [MockOrganization2, MockDefaultOrganization],
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const organizationPicker = canvas.getByRole("button", {
			name: "Organization: My Organization",
		});
		await expect(organizationPicker).toBeVisible();

		const input = canvas.getByRole("textbox", { name: "Chat message" });
		await userEvent.click(input);
		await userEvent.keyboard("hello world");
		await expect(
			canvas.getByRole("button", {
				name: "Organization: My Organization",
			}),
		).toBeVisible();
	},
};

export const RestrictedMultiOrganizationUser: Story = {
	parameters: {
		showOrganizations: true,
		organizations: [MockDefaultOrganization, MockOrganization2],
	},
	beforeEach: () => {
		spyOn(API, "getOrganizations").mockResolvedValue([
			MockDefaultOrganization,
			MockOrganization2,
		]);
		// Model agents-access: "me" supplies the owner for member-scoped chat:create.
		spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) =>
			Object.fromEntries(
				Object.entries(checks).map(([id, check]) => [
					id,
					check.object.owner_id === "me" &&
						check.object.organization_id === MockOrganization2.id,
				]),
			),
		);
	},
	args: {
		...defaultArgs,
		onCreateChat: fn().mockResolvedValue(undefined),
	},
	play: async ({ canvasElement, args }) => {
		await submitMessage(canvasElement, "test message");
		await waitFor(() => {
			expect(args.onCreateChat).toHaveBeenCalledWith(
				expect.objectContaining({
					organizationId: MockOrganization2.id,
				}),
			);
		});
	},
};

export const RestrictedUserKeepsPersistedWorkspace: Story = {
	parameters: {
		showOrganizations: true,
		organizations: [MockDefaultOrganization, MockOrganization2],
	},
	args: {
		...defaultArgs,
		onCreateChat: fn().mockResolvedValue(undefined),
		workspaceOptions: [
			{
				...MockWorkspace,
				id: "ws-permitted-org",
				name: "permitted-workspace",
				organization_id: MockOrganization2.id,
			},
		],
		workspaceCount: 1,
	},
	beforeEach: () => {
		localStorage.setItem("agents.selected-workspace-id", "ws-permitted-org");
		mockPermittedOrganizations({
			[MockDefaultOrganization.id]: false,
			[MockOrganization2.id]: true,
		});
	},
	play: async ({ canvasElement, args }) => {
		await submitMessage(canvasElement, "test message");
		await waitFor(() => {
			expect(args.onCreateChat).toHaveBeenCalledWith(
				expect.objectContaining({
					organizationId: MockOrganization2.id,
					workspaceId: "ws-permitted-org",
				}),
			);
		});
	},
};

export const RestrictedUserKeepsPersistedAttachments: Story = {
	parameters: {
		showOrganizations: true,
		organizations: [MockDefaultOrganization, MockOrganization2],
	},
	beforeEach: () => {
		localStorage.clear();
		localStorage.setItem(
			"agents.persisted-attachments",
			JSON.stringify([
				{
					fileId: "file-permitted-org",
					fileName: "notes.txt",
					fileType: "text/plain",
					lastModified: 1700000000000,
					organizationId: MockOrganization2.id,
				},
			]),
		);
		mockPermittedOrganizations({
			[MockDefaultOrganization.id]: false,
			[MockOrganization2.id]: true,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(canvas.getByLabelText("Remove notes.txt")).toBeInTheDocument();
		});
		const stored = localStorage.getItem("agents.persisted-attachments");
		expect(stored).toContain("file-permitted-org");
	},
};

export const OrganizationAuthorizationFailure: Story = {
	parameters: {
		showOrganizations: true,
		organizations: [MockDefaultOrganization, MockOrganization2],
	},
	beforeEach: () => {
		localStorage.clear();
		localStorage.setItem(emptyInputStorageKey, "draft message");
		spyOn(API, "getOrganizations").mockResolvedValue([
			MockDefaultOrganization,
			MockOrganization2,
		]);
		spyOn(API, "checkAuthorization").mockRejectedValue(
			new Error("authorization check failed"),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findAllByText(/authorization check failed/i);
		expect(canvas.getByRole("button", { name: "Send" })).toBeDisabled();
	},
};

export const LoadingWorkspacesBlocksSendUntilValidated: Story = {
	parameters: {
		showOrganizations: true,
		organizations: [MockDefaultOrganization, MockOrganization2],
	},
	args: {
		...defaultArgs,
		workspaceOptions: [],
		isWorkspacesLoading: true,
	},
	beforeEach: () => {
		localStorage.setItem(emptyInputStorageKey, "draft message");
		localStorage.setItem("agents.selected-workspace-id", "ws-default-org");
		mockPermittedOrganizations({
			[MockDefaultOrganization.id]: true,
			[MockOrganization2.id]: true,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Wait for permissions to settle before checking workspace validation.
		await canvas.findByRole("button", {
			name: "Organization: My Organization",
		});
		await expect(canvas.getByRole("button", { name: "Send" })).toBeDisabled();
	},
};

export const DelayedOrganizationAuthorization: Story = {
	parameters: {
		showOrganizations: true,
		organizations: [MockDefaultOrganization, MockOrganization2],
	},
	beforeEach: () => {
		localStorage.setItem(emptyInputStorageKey, "draft message");
		mockPermittedOrganizations(
			{
				[MockDefaultOrganization.id]: true,
				[MockOrganization2.id]: true,
			},
			1_500,
		);
	},
	args: {
		...defaultArgs,
		workspaceOptions: [
			{
				...MockWorkspace,
				id: "ws-provisional",
				name: "provisional-workspace",
				organization_id: MockDefaultOrganization.id,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const sendButton = canvas.getByRole("button", { name: "Send" });
		await expect(sendButton).toBeDisabled();
		await expect(
			canvas.getByRole("button", { name: "More options" }),
		).toBeDisabled();
		// dispatchEvent returns false when a handler accepted the drop
		// via preventDefault, giving a race-free accepted/ignored signal.
		const dropFile = (name: string): boolean => {
			const dataTransfer = new DataTransfer();
			dataTransfer.items.add(new File(["hello"], name, { type: "text/plain" }));
			return canvas.getByTestId("chat-composer").dispatchEvent(
				new DragEvent("drop", {
					bubbles: true,
					cancelable: true,
					dataTransfer,
				}),
			);
		};
		// Pending authorization leaves attachments without a valid org, so drops
		// must be ignored.
		expect(dropFile("drop.txt")).toBe(true);
		expect(canvas.queryByLabelText("Remove drop.txt")).not.toBeInTheDocument();
		// The pending option list is unfiltered, so the picker must stay hidden.
		expect(
			canvas.queryByRole("button", { name: /organization/i }),
		).not.toBeInTheDocument();
		await waitFor(() => expect(sendButton).toBeEnabled(), { timeout: 3_000 });
		await canvas.findByRole("button", { name: /organization/i });
		// Positive control: once settled the same drop is accepted and
		// attaches, so the pending-state assertions exercised a real path.
		expect(dropFile("after.txt")).toBe(false);
		await waitFor(() =>
			expect(canvas.getByLabelText("Remove after.txt")).toBeInTheDocument(),
		);
	},
};

// Mutable permissions let play functions change authorization across refetches.
// The story-local QueryClient exposes those refetches; the preview client's
// instance is inaccessible and uses infinite stale time.
const revocablePermissions: Record<string, boolean> = {};
let revocableQueryClient: QueryClient | undefined;

const withRevocableQueryClient: Decorator = (Story) => {
	const [queryClient] = useState(
		() =>
			new QueryClient({
				defaultOptions: {
					queries: {
						staleTime: Number.POSITIVE_INFINITY,
						retry: false,
					},
				},
			}),
	);
	revocableQueryClient = queryClient;
	return (
		<QueryClientProvider client={queryClient}>
			<Story />
		</QueryClientProvider>
	);
};

const mockRevocablePermissions = (permissions: Record<string, boolean>) => {
	for (const key of Object.keys(revocablePermissions)) {
		delete revocablePermissions[key];
	}
	Object.assign(revocablePermissions, permissions);
	spyOn(API, "getOrganizations").mockResolvedValue([
		MockDefaultOrganization,
		MockOrganization2,
	]);
	spyOn(API, "checkAuthorization").mockImplementation(async () => ({
		...revocablePermissions,
	}));
};

const revocableStoryContext = {
	parameters: {
		showOrganizations: true,
		organizations: [MockDefaultOrganization, MockOrganization2],
	},
	decorators: [withRevocableQueryClient],
};

const allOrganizationsPermitted = {
	[MockDefaultOrganization.id]: true,
	[MockOrganization2.id]: true,
};

export const RevokedSelectionDoesNotResurrect: Story = {
	...revocableStoryContext,
	beforeEach: () => {
		mockRevocablePermissions(allOrganizationsPermitted);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = await canvas.findByRole("button", {
			name: "Organization: My Organization",
		});
		await userEvent.click(trigger);
		await userEvent.click(
			await screen.findByRole("option", { name: /My Organization 2/ }),
		);
		await canvas.findByRole("button", {
			name: "Organization: My Organization 2",
		});

		revocablePermissions[MockOrganization2.id] = false;
		await revocableQueryClient?.invalidateQueries();
		await waitFor(() =>
			expect(
				canvas.queryByRole("button", { name: /organization/i }),
			).not.toBeInTheDocument(),
		);

		revocablePermissions[MockOrganization2.id] = true;
		await revocableQueryClient?.invalidateQueries();
		await canvas.findByRole("button", {
			name: "Organization: My Organization",
		});
	},
};

export const RevokedOrgChangeClearsStoredWorkspace: Story = {
	...revocableStoryContext,
	args: {
		...defaultArgs,
		workspaceOptions: [
			{
				...MockWorkspace,
				id: "ws-default-org",
				name: "default-workspace",
				organization_id: MockDefaultOrganization.id,
			},
		],
		workspaceCount: 1,
	},
	beforeEach: () => {
		localStorage.setItem("agents.selected-workspace-id", "ws-default-org");
		mockRevocablePermissions(allOrganizationsPermitted);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() =>
			expect(
				canvas.getByLabelText("Remove workspace default-workspace"),
			).toBeInTheDocument(),
		);

		revocablePermissions[MockDefaultOrganization.id] = false;
		await revocableQueryClient?.invalidateQueries();
		await waitFor(() =>
			expect(
				canvas.queryByLabelText("Remove workspace default-workspace"),
			).not.toBeInTheDocument(),
		);

		revocablePermissions[MockDefaultOrganization.id] = true;
		await revocableQueryClient?.invalidateQueries();
		await canvas.findByRole("button", {
			name: "Organization: My Organization 2",
		});
		expect(
			canvas.queryByLabelText("Remove workspace default-workspace"),
		).not.toBeInTheDocument();
		expect(localStorage.getItem("agents.selected-workspace-id")).toBeNull();
	},
};

export const EmptyPermittedSetPreservesStoredWorkspace: Story = {
	...revocableStoryContext,
	args: {
		...defaultArgs,
		workspaceOptions: [
			{
				...MockWorkspace,
				id: "ws-org-2",
				name: "org2-workspace",
				organization_id: MockOrganization2.id,
			},
		],
	},
	beforeEach: () => {
		localStorage.setItem("agents.selected-workspace-id", "ws-org-2");
		mockRevocablePermissions({
			[MockDefaultOrganization.id]: false,
			[MockOrganization2.id]: true,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByLabelText("Remove workspace org2-workspace");

		revocablePermissions[MockOrganization2.id] = false;
		await revocableQueryClient?.invalidateQueries();
		await canvas.findByText(/don't have permission/i);

		revocablePermissions[MockOrganization2.id] = true;
		await revocableQueryClient?.invalidateQueries();
		await canvas.findByLabelText("Remove workspace org2-workspace");
		expect(localStorage.getItem("agents.selected-workspace-id")).toBe(
			"ws-org-2",
		);
	},
};

export const SingleOrgIgnoresStalePermittedCache: Story = {
	parameters: {
		showOrganizations: false,
		organizations: [MockDefaultOrganization],
		queries: [
			{
				key: permittedOrgsKey,
				data: [MockOrganization2],
			},
		],
	},
	args: {
		...defaultArgs,
		onCreateChat: fn().mockResolvedValue(undefined),
	},
	play: async ({ canvasElement, args }) => {
		await submitMessage(canvasElement, "test message");
		await waitFor(() => {
			expect(args.onCreateChat).toHaveBeenCalledWith(
				expect.objectContaining({
					organizationId: MockDefaultOrganization.id,
				}),
			);
		});
	},
};

export const RevokedPendingOrgClosesConfirmDialog: Story = {
	...revocableStoryContext,
	beforeEach: () => {
		localStorage.clear();
		localStorage.setItem(
			persistedAttachmentsStorageKey,
			JSON.stringify([
				{
					fileId: "file-default-org",
					fileName: "notes.txt",
					fileType: "text/plain",
					lastModified: 1700000000000,
					organizationId: MockDefaultOrganization.id,
				},
			]),
		);
		mockRevocablePermissions(allOrganizationsPermitted);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await waitFor(() =>
			expect(canvas.getByLabelText("Remove notes.txt")).toBeInTheDocument(),
		);
		await userEvent.click(
			await canvas.findByRole("button", {
				name: "Organization: My Organization",
			}),
		);
		await userEvent.click(
			await screen.findByRole("option", { name: /My Organization 2/ }),
		);
		await body.findByText(
			"Changing organization will remove your current attachments.",
		);

		revocablePermissions[MockOrganization2.id] = false;
		await revocableQueryClient?.invalidateQueries();
		await waitFor(() =>
			expect(
				body.queryByText(
					"Changing organization will remove your current attachments.",
				),
			).not.toBeInTheDocument(),
		);
		expect(canvas.getByLabelText("Remove notes.txt")).toBeInTheDocument();
	},
};

export const OrgPickerTightSpacing: Story = {
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
		const orgTrigger = await canvas.findByTestId("compact-org-selector");
		const composer = await canvas.findByTestId("chat-composer");

		const orgRect = orgTrigger.getBoundingClientRect();
		const composerRect = composer.getBoundingClientRect();
		const gap = composerRect.top - orgRect.bottom;
		expect(gap).toBeGreaterThanOrEqual(0);
		expect(gap).toBeLessThan(16);
	},
};

/**
 * Standalone story for the org-change confirmation dialog. Renders
 * the ConfirmDialog directly in its open state, following the same
 * pattern as DeleteConfirmationDialog in AgentsPageLayout.stories.
 */
export const OrgChangeConfirmation: Story = {
	render: () => (
		<ConfirmDialog
			open
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
	},
	args: {
		...defaultArgs,
		onCreateChat: fn().mockResolvedValue(undefined),
	},
	beforeEach: () => {
		localStorage.clear();
		// Another org's persisted attachment must survive a visit while
		// the user has no chat permission anywhere.
		localStorage.setItem(
			persistedAttachmentsStorageKey,
			JSON.stringify([
				{
					fileId: "file-other-org",
					fileName: "keep.txt",
					fileType: "text/plain",
					lastModified: 1000,
					organizationId: MockOrganization2.id,
				},
			]),
		);
		mockPermittedOrganizations({
			[MockDefaultOrganization.id]: false,
			[MockOrganization2.id]: false,
		});
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await waitFor(
			() => {
				expect(canvas.getByText(/don't have permission/i)).toBeInTheDocument();
			},
			{ timeout: 3000 },
		);
		expect(canvas.getByRole("button", { name: "Send" })).toBeDisabled();
		expect(args.onCreateChat).not.toHaveBeenCalled();
		expect(
			localStorage.getItem(persistedAttachmentsStorageKey) ?? "",
		).toContain("file-other-org");
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

/**
 * Member-scoped roles like agents-access grant chat:create only on
 * chats the user owns, so the per-org check must carry owner context
 * for the picker to render.
 */
export const MemberScopedPermissionsShowOrgPicker: Story = {
	parameters: {
		showOrganizations: true,
		organizations: [MockDefaultOrganization, MockOrganization2],
	},
	beforeEach: () => {
		spyOn(API, "getOrganizations").mockResolvedValue([
			MockDefaultOrganization,
			MockOrganization2,
		]);
		spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) =>
			Object.fromEntries(
				Object.entries(checks).map(([id, check]) => [
					id,
					check.object.owner_id === "me",
				]),
			),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const picker = await canvas.findByRole(
			"button",
			{ name: /^Organization:/ },
			{ timeout: 3000 },
		);
		await userEvent.click(picker);
		await screen.findByRole("option", {
			name: MockDefaultOrganization.display_name,
		});
		await userEvent.click(
			screen.getByRole("option", { name: MockOrganization2.display_name }),
		);
		expect(
			canvas.getByRole("button", {
				name: `Organization: ${MockOrganization2.display_name}`,
			}),
		).toBeInTheDocument();
	},
};

export const ClaudeCodeRuntimeSubmission: Story = {
	args: {
		...defaultArgs,
		onCreateChat: fn().mockResolvedValue(undefined),
		claudeCodeOrgIds: new Set([MockDefaultOrganization.id]),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "More options" }));
		const item = await screen.findByRole("menuitemcheckbox", {
			name: /Run with Claude Code/,
		});
		await userEvent.click(item);

		expect(await canvas.findByText("Claude Code")).toBeVisible();
		expect(canvas.getByRole("combobox", { name: "Default" })).toBeVisible();

		await submitMessage(canvasElement, "build me a server");
		await waitFor(() => {
			expect(args.onCreateChat).toHaveBeenCalled();
		});
		const options = getCreateOptions(args.onCreateChat);
		expect(options.runtime).toBe("claude_code");
		expect(options.model).toBeUndefined();
		expect(options.workspaceId).toBeUndefined();
	},
};

export const ClaudeCodeRuntimeModelPick: Story = {
	args: {
		...defaultArgs,
		onCreateChat: fn().mockResolvedValue(undefined),
		claudeCodeOrgIds: new Set([MockDefaultOrganization.id]),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "More options" }));
		await userEvent.click(
			await screen.findByRole("menuitemcheckbox", {
				name: /Run with Claude Code/,
			}),
		);

		await userEvent.click(
			await canvas.findByRole("combobox", { name: "Default" }),
		);
		const body = within(document.body);
		expect(
			body.queryByRole("option", { name: /GPT-4o/ }),
		).not.toBeInTheDocument();
		await userEvent.click(
			await body.findByRole("option", { name: /Claude Sonnet 4/ }),
		);

		await submitMessage(canvasElement, "build me a server");
		await waitFor(() => {
			expect(args.onCreateChat).toHaveBeenCalled();
		});
		const options = getCreateOptions(args.onCreateChat);
		expect(options.runtime).toBe("claude_code");
		expect(options.model).toBe(claudeModelConfigID);
	},
};

export const ClaudeCodeStillGatedWithoutGateway: Story = {
	args: {
		...defaultArgs,
		aiGatewayDisabled: true,
		claudeCodeOrgIds: new Set([MockDefaultOrganization.id]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("textbox")).toHaveAttribute(
			"aria-disabled",
			"true",
		);
	},
};

export const ClaudeCodeHiddenWithoutOrgConfig: Story = {
	args: {
		...defaultArgs,
		claudeCodeOrgIds: new Set<string>(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "More options" }));
		await screen.findByRole("menuitemcheckbox", { name: /Plan first/ });
		expect(
			screen.queryByRole("menuitemcheckbox", {
				name: /Run with Claude Code/,
			}),
		).not.toBeInTheDocument();
	},
};
