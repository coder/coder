import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { delay } from "msw";
import { type ComponentProps, useEffect, useState } from "react";
import { QueryClient, QueryClientProvider, useQueryClient } from "react-query";
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
import { aiProvidersListKey } from "#/api/queries/aiProviders";
import {
	organizationChatModelsKey,
	userChatPersonalModelOverrides,
	userChatProviderConfigsKey,
} from "#/api/queries/chats";
import { permittedOrganizationsKey } from "#/api/queries/organizations";
import type * as TypesGen from "#/api/typesGenerated";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import { createDeferred, type Deferred } from "#/testHelpers/deferred";
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
	emptyInputStorageKey,
	selectedOrganizationIdStorageKey,
} from "./AgentCreateForm";

let pendingOrganizationAuthorization: Deferred<
	Awaited<ReturnType<typeof API.checkAuthorization>>
>;
let capturedQueryClient: QueryClient | undefined;

const permittedOrgsKey = permittedOrganizationsKey({
	object: { resource_type: "chat", owner_id: "me" },
	action: "create",
});

const modelID = "model-config-1";
const claudeModelConfigID = "model-config-claude";

const buildModelConfig = (
	overrides: Partial<TypesGen.ChatModel> = {},
): TypesGen.ChatModel => ({
	...MockChatModel,
	id: modelID,
	organization_id: MockDefaultOrganization.id,
	model: "gpt-4o",
	display_name: "GPT-4o",
	created_at: "2026-02-18T00:00:00.000Z",
	updated_at: "2026-02-18T00:00:00.000Z",
	...overrides,
});

const defaultModelConfigs: TypesGen.ChatModel[] = [
	buildModelConfig({ is_default: true }),
	buildModelConfig({
		id: claudeModelConfigID,
		ai_provider_id: "provider-anthropic",
		model: "claude-sonnet-4",
		display_name: "Claude Sonnet 4",
		context_limit: 200_000,
	}),
];

// Model catalog with both providers available, matching defaultModelConfigs.
const organization2ModelConfig = buildModelConfig({
	id: "model-config-org-2",
	organization_id: MockOrganization2.id,
	model: "gpt-4.1-mini",
	display_name: "GPT 4.1 Mini",
	is_default: true,
});

const defaultModelCatalog: TypesGen.OrganizationChatModelsResponse = {
	models: defaultModelConfigs,
	providers: [
		MockChatModelProviderDescriptor,
		{
			...MockChatModelProviderDescriptor,
			id: "provider-anthropic",
			type: "anthropic",
			display_name: "Anthropic",
		},
	],
	unsupported_providers: [],
};

const organization2RuntimeCatalog: TypesGen.OrganizationChatModelsResponse = {
	...defaultModelCatalog,
	models: [organization2ModelConfig, ...defaultModelConfigs],
};

const organization2LocalCatalog: TypesGen.OrganizationChatModelsResponse = {
	...defaultModelCatalog,
	models: [organization2ModelConfig],
};

const organization2ForeignOnlyCatalog: TypesGen.OrganizationChatModelsResponse =
	{
		...defaultModelCatalog,
		models: defaultModelConfigs,
	};

const userApiKeyRequiredCatalog: TypesGen.OrganizationChatModelsResponse = {
	...defaultModelCatalog,
	providers: defaultModelCatalog.providers.map((provider) => ({
		...provider,
		available: false,
		unavailable_reason: "user_api_key_required",
	})),
};

const missingAPIKeyCatalog: TypesGen.OrganizationChatModelsResponse = {
	...defaultModelCatalog,
	providers: defaultModelCatalog.providers.map((provider) => ({
		...provider,
		available: false,
		unavailable_reason: "missing_api_key",
	})),
};

const fetchFailedCatalog: TypesGen.OrganizationChatModelsResponse = {
	...defaultModelCatalog,
	providers: defaultModelCatalog.providers.map((provider) => ({
		...provider,
		available: false,
		unavailable_reason: "fetch_failed",
	})),
};

const unsupportedProviderCatalog: TypesGen.OrganizationChatModelsResponse = {
	models: [],
	providers: [
		{
			...MockChatModelProviderDescriptor,
			id: "provider-copilot",
			type: "copilot",
			display_name: "GitHub Copilot",
			has_api_key: false,
			has_effective_api_key: false,
			available: false,
		},
	],
	unsupported_providers: [
		{ provider: "copilot", display_name: "GitHub Copilot" },
	],
};

const unsupportedProviderWithDisabledSupportedCatalog: TypesGen.OrganizationChatModelsResponse =
	{
		...unsupportedProviderCatalog,
		providers: [
			...unsupportedProviderCatalog.providers,
			{
				...MockChatModelProviderDescriptor,
				id: "provider-anthropic",
				type: "anthropic",
				display_name: "Anthropic",
				enabled: false,
				available: false,
			},
		],
	};

const defaultUserProviderConfigs: TypesGen.UserChatProviderConfig[] = [
	{
		provider_id: "provider-1",
		provider: "openai",
		display_name: "OpenAI",
		icon: "",
		enabled: true,
		has_user_api_key: false,
		has_central_api_key_fallback: true,
		byok_enabled: false,
	},
	{
		provider_id: "provider-anthropic",
		provider: "anthropic",
		display_name: "Anthropic",
		icon: "",
		enabled: true,
		has_user_api_key: false,
		has_central_api_key_fallback: true,
		byok_enabled: false,
	},
];

const buildRootPersonalModelOverride = (
	overrides: Partial<TypesGen.ChatPersonalModelOverride> = {},
): TypesGen.ChatPersonalModelOverride => ({
	context: "root",
	mode: "chat_default",
	model_config_id: "",
	is_set: true,
	...overrides,
});

const buildPersonalModelOverridesResponse = (
	root = buildRootPersonalModelOverride({ is_set: false }),
): TypesGen.UserChatPersonalModelOverridesResponse => ({
	enabled: true,
	root,
	general: {
		context: "general",
		mode: "deployment_default",
		model_config_id: "",
		is_set: false,
	},
	explore: {
		context: "explore",
		mode: "deployment_default",
		model_config_id: "",
		is_set: false,
	},
	deployment_defaults: {
		general: { context: "general", model_config_id: "" },
		explore: { context: "explore", model_config_id: "" },
	},
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
		workspaceCount: 0,
		workspaceOptions: [],
		workspacesError: undefined,
		isWorkspacesLoading: false,
	},
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: defaultModelCatalog,
			},
			{
				key: userChatProviderConfigsKey,
				data: defaultUserProviderConfigs,
			},
			{
				key: userChatPersonalModelOverrides(MockDefaultOrganization.id)
					.queryKey,
				data: buildPersonalModelOverridesResponse(),
			},
		],
	},
	beforeEach: () => {
		localStorage.clear();
		spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
		// Stories that replace parameters.queries lose the seeded overrides
		// entry above; resolve the API for any organization so the send gate
		// (blocked until overrides resolve) does not stall unrelated stories.
		spyOn(
			API.experimental,
			"getUserChatPersonalModelOverrides",
		).mockResolvedValue(buildPersonalModelOverridesResponse());
	},
};

export default meta;
type Story = StoryObj<typeof AgentCreateForm>;

const defaultArgs = meta.args;

const RemountAgentCreateForm = (
	props: ComponentProps<typeof AgentCreateForm>,
) => {
	const [key, setKey] = useState(0);
	return (
		<>
			<button type="button" onClick={() => setKey((current) => current + 1)}>
				Remount form
			</button>
			<AgentCreateForm key={key} {...props} />
		</>
	);
};

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
	const sendButton = canvas.getByRole("button", { name: "Send" });
	await waitFor(() => expect(sendButton).toBeEnabled());
	await userEvent.click(sendButton);
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
	reasoningEffort?: string;
};

export const RootPersonalModelOverrideModelSelected: Story = {
	args: {
		...defaultArgs,
		onCreateChat: fn().mockResolvedValue(undefined),
	},
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: defaultModelCatalog,
			},
			{
				key: userChatProviderConfigsKey,
				data: defaultUserProviderConfigs,
			},
			{
				key: userChatPersonalModelOverrides(MockDefaultOrganization.id)
					.queryKey,
				data: buildPersonalModelOverridesResponse(
					buildRootPersonalModelOverride({
						mode: "model",
						model_config_id: claudeModelConfigID,
					}),
				),
			},
		],
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByRole("combobox", { name: "Claude Sonnet 4" }),
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
	},
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: defaultModelCatalog,
			},
			{
				key: userChatProviderConfigsKey,
				data: defaultUserProviderConfigs,
			},
			{
				key: userChatPersonalModelOverrides(MockDefaultOrganization.id)
					.queryKey,
				data: buildPersonalModelOverridesResponse(
					buildRootPersonalModelOverride({
						mode: "chat_default",
						model_config_id: "",
					}),
				),
			},
		],
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
		expect(getCreateOptions(args.onCreateChat).model).toBe(modelID);
	},
};

export const RootOverrideMissingFromCatalog: Story = {
	args: {
		...defaultArgs,
		onCreateChat: fn().mockResolvedValue(undefined),
	},
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: defaultModelCatalog,
			},
			{
				key: userChatProviderConfigsKey,
				data: defaultUserProviderConfigs,
			},
			{
				key: userChatPersonalModelOverrides(MockDefaultOrganization.id)
					.queryKey,
				data: buildPersonalModelOverridesResponse(
					buildRootPersonalModelOverride({
						mode: "model",
						model_config_id: "model-does-not-exist",
						is_set: true,
					}),
				),
			},
		],
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
		expect(getCreateOptions(args.onCreateChat).model).toBe(modelID);
	},
};

export const LastUsedModelFallbackWithoutRootOverride: Story = {
	args: {
		...defaultArgs,
		onCreateChat: fn().mockResolvedValue(undefined),
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
	},
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: defaultModelCatalog,
			},
			{
				key: userChatProviderConfigsKey,
				data: defaultUserProviderConfigs,
			},
			{
				key: userChatPersonalModelOverrides(MockDefaultOrganization.id)
					.queryKey,
				data: buildPersonalModelOverridesResponse(
					buildRootPersonalModelOverride({
						mode: "chat_default",
						model_config_id: "",
					}),
				),
			},
		],
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

// Model configs with reasoning effort bounds configured. GPT-4o uses the
// full global scale; Claude is capped at medium.
const effortModelConfigs: TypesGen.ChatModel[] = [
	buildModelConfig({
		is_default: true,
		model_config: { reasoning_effort: { default: "medium" } },
		reasoning_efforts: [
			"none",
			"minimal",
			"low",
			"medium",
			"high",
			"xhigh",
			"max",
		],
	}),
	buildModelConfig({
		id: claudeModelConfigID,
		ai_provider_id: "provider-anthropic",
		model: "claude-sonnet-4",
		display_name: "Claude Sonnet 4",
		context_limit: 200_000,
		model_config: { reasoning_effort: { default: "low" } },
		reasoning_efforts: ["low", "medium"],
	}),
];

const effortModelCatalog: TypesGen.OrganizationChatModelsResponse = {
	...defaultModelCatalog,
	models: effortModelConfigs,
};

const limitedEffortModelCatalog: TypesGen.OrganizationChatModelsResponse = {
	...defaultModelCatalog,
	models: [
		buildModelConfig({
			is_default: true,
			model_config: { reasoning_effort: { default: "low" } },
			reasoning_efforts: ["low", "medium"],
		}),
	],
};

export const RemembersReasoningEffortByModel: Story = {
	args: {
		...defaultArgs,
	},
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: effortModelCatalog,
			},
			{
				key: userChatProviderConfigsKey,
				data: defaultUserProviderConfigs,
			},
		],
	},
	beforeEach: () => {
		localStorage.clear();
		saveReasoningEffortForModel(modelID, "high");
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
			expect(getReasoningEffortForModel(modelID)).toBe("xhigh");
		});
		await userEvent.keyboard("{Escape}");
	},
};

export const PersistedReasoningEffortOutranksRootOverride: Story = {
	args: {
		...defaultArgs,
		onCreateChat: fn().mockResolvedValue(undefined),
	},
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: effortModelCatalog,
			},
			{
				key: userChatProviderConfigsKey,
				data: defaultUserProviderConfigs,
			},
			{
				key: userChatPersonalModelOverrides(MockDefaultOrganization.id)
					.queryKey,
				data: buildPersonalModelOverridesResponse(
					buildRootPersonalModelOverride({
						mode: "model",
						model_config_id: modelID,
						reasoning_effort: "high",
					}),
				),
			},
		],
	},
	beforeEach: () => {
		localStorage.clear();
		saveReasoningEffortForModel(modelID, "low");
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
	},
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: effortModelCatalog,
			},
			{
				key: userChatProviderConfigsKey,
				data: defaultUserProviderConfigs,
			},
			{
				key: userChatPersonalModelOverrides(MockDefaultOrganization.id)
					.queryKey,
				data: buildPersonalModelOverridesResponse(
					buildRootPersonalModelOverride({
						mode: "model",
						model_config_id: modelID,
						reasoning_effort: "high",
					}),
				),
			},
		],
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
	},
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: limitedEffortModelCatalog,
			},
			{
				key: userChatProviderConfigsKey,
				data: defaultUserProviderConfigs,
			},
			{
				key: userChatPersonalModelOverrides(MockDefaultOrganization.id)
					.queryKey,
				data: buildPersonalModelOverridesResponse(
					buildRootPersonalModelOverride({
						mode: "model",
						model_config_id: modelID,
						reasoning_effort: "medium",
					}),
				),
			},
		],
	},
	beforeEach: () => {
		localStorage.clear();
		saveReasoningEffortForModel(modelID, "max");
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
	parameters: {
		pixel: { exclude: true },
		queries: [
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: effortModelCatalog,
			},
			{
				key: userChatProviderConfigsKey,
				data: defaultUserProviderConfigs,
			},
		],
	},
	args: {
		...defaultArgs,
		onCreateChat: fn().mockResolvedValue(undefined),
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
		expect(options.model).toBe(modelID);
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
	// Leave the model queries unseeded so they stay pending.
	parameters: { queries: [] },
	args: {
		...defaultArgs,
	},
};

export const CachedModelsWithRefetchError: Story = {
	decorators: [
		(Story) => {
			const queryClient = useQueryClient();
			useEffect(() => {
				void queryClient.invalidateQueries({
					queryKey: organizationChatModelsKey(MockDefaultOrganization.id),
					exact: true,
				});
			}, [queryClient]);
			return <Story />;
		},
	],
	beforeEach: () => {
		spyOn(API.experimental, "getChatModels").mockRejectedValueOnce(
			new Error("Failed to refresh available models."),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText("Failed to refresh available models."),
		).toBeVisible();
		expect(canvas.getByRole("combobox", { name: "GPT-4o" })).toBeVisible();
	},
};

export const LoadingPersonalModelOverrides: Story = {
	args: {
		...defaultArgs,
	},
	beforeEach: () => {
		spyOn(
			API.experimental,
			"getUserChatPersonalModelOverrides",
		).mockReturnValue(new Promise(() => undefined));
	},
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: defaultModelCatalog,
			},
			{
				key: userChatProviderConfigsKey,
				data: defaultUserProviderConfigs,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("textbox")).toHaveAttribute(
			"aria-disabled",
			"true",
		);
	},
};

export const FailedPersonalModelOverridesBlocksSend: Story = {
	args: {
		...defaultArgs,
	},
	beforeEach: () => {
		spyOn(
			API.experimental,
			"getUserChatPersonalModelOverrides",
		).mockRejectedValue(new Error("failed to load personal overrides"));
	},
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: defaultModelCatalog,
			},
			{
				key: userChatProviderConfigsKey,
				data: defaultUserProviderConfigs,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// A failed override fetch must keep sending blocked: submitting would
		// pass a catalog fallback as an explicit model, silently bypassing the
		// user's saved root override.
		await canvas.findAllByText(/failed to load personal overrides/i);
		await expect(canvas.getByRole("textbox")).toHaveAttribute(
			"aria-disabled",
			"true",
		);
	},
};

const emptyModelCatalog: TypesGen.OrganizationChatModelsResponse = {
	models: [],
	providers: [],
	unsupported_providers: [],
};

export const NoModelsConfigured: Story = {
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: emptyModelCatalog,
			},
			{
				key: userChatProviderConfigsKey,
				data: defaultUserProviderConfigs,
			},
		],
	},
	args: {
		...defaultArgs,
	},
};

export const ProviderRequiresUserApiKey: Story = {
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: userApiKeyRequiredCatalog,
			},
			{
				key: userChatProviderConfigsKey,
				data: defaultUserProviderConfigs,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/AI models aren't available yet/)).toBeVisible();
		expect(canvas.getByRole("link", { name: "Settings" })).toHaveAttribute(
			"href",
			"/agents/settings/api-keys",
		);
	},
};

export const ProviderMissingAPIKey: Story = {
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: missingAPIKeyCatalog,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/AI models aren't available yet/)).toBeVisible();
	},
};

export const ProviderFetchFailed: Story = {
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: fetchFailedCatalog,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/AI models aren't available yet/)).toBeVisible();
	},
};

export const UnsupportedProviderOnly: Story = {
	args: { ...defaultArgs, canConfigureAgentSetup: true },
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: unsupportedProviderCatalog,
			},
			{ key: aiProvidersListKey, data: [] },
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/GitHub Copilot is configured but/i)).toBeVisible();
		expect(
			canvas.getByRole("link", { name: "not supported by Coder Agents" }),
		).toBeVisible();
	},
};

export const UnsupportedProviderAndDisabledSupportedProvider: Story = {
	args: { ...defaultArgs, canConfigureAgentSetup: true },
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: unsupportedProviderWithDisabledSupportedCatalog,
			},
			{ key: aiProvidersListKey, data: [] },
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/GitHub Copilot is configured but/i)).toBeVisible();
		expect(
			canvas.getByRole("link", { name: "not supported by Coder Agents" }),
		).toBeVisible();
	},
};

export const MissingProviderAndModelSetup: Story = {
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: emptyModelCatalog,
			},
			{
				key: userChatProviderConfigsKey,
				data: defaultUserProviderConfigs,
			},
			{ key: aiProvidersListKey, data: [] },
		],
	},
	args: {
		...defaultArgs,
		canConfigureAgentSetup: true,
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
		await waitFor(() =>
			expect(textbox).not.toHaveAttribute("aria-disabled", "true"),
		);
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
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: defaultModelCatalog,
			},
			{
				key: organizationChatModelsKey(MockOrganization2.id),
				data: organization2RuntimeCatalog,
			},
			{
				key: userChatProviderConfigsKey,
				data: defaultUserProviderConfigs,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const organizationTrigger = await canvas.findByRole("button", {
			name: `Organization: ${MockDefaultOrganization.display_name}`,
		});
		expect(canvas.getByRole("combobox", { name: "GPT-4o" })).toBeVisible();

		const input = canvas.getByRole("textbox", { name: "Chat message" });
		await userEvent.click(input);
		await userEvent.keyboard("hello world");
		await expect(organizationTrigger).toBeVisible();

		await userEvent.click(organizationTrigger);
		await userEvent.click(
			await body.findByRole("option", {
				name: new RegExp(MockOrganization2.display_name),
			}),
		);

		await waitFor(() => {
			expect(
				canvas.getByRole("button", {
					name: `Organization: ${MockOrganization2.display_name}`,
				}),
			).toBeVisible();
			expect(
				canvas.getByRole("combobox", { name: "GPT 4.1 Mini" }),
			).toBeVisible();
		});

		await userEvent.keyboard("{Escape}");
		await userEvent.click(
			canvas.getByRole("combobox", { name: "GPT 4.1 Mini" }),
		);
		await waitFor(() => {
			const visibleOptions = body
				.getAllByRole("option")
				.filter((option) => option.checkVisibility());
			expect(
				visibleOptions.some((option) =>
					option.textContent?.includes("GPT 4.1 Mini"),
				),
			).toBe(true);
			expect(
				visibleOptions.some((option) => option.textContent?.includes("GPT-4o")),
			).toBe(false);
		});
		expect(
			canvas.queryByRole("combobox", { name: "GPT-4o" }),
		).not.toBeInTheDocument();
	},
};

export const DelayedAuthorizationPreservesForeignPersistedModel: Story = {
	parameters: {
		showOrganizations: true,
		organizations: [MockDefaultOrganization, MockOrganization2],
		queries: [
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: defaultModelCatalog,
			},
			{
				key: organizationChatModelsKey(MockOrganization2.id),
				data: organization2LocalCatalog,
			},
			{
				key: userChatProviderConfigsKey,
				data: defaultUserProviderConfigs,
			},
		],
	},
	beforeEach: () => {
		localStorage.clear();
		localStorage.setItem(
			"agents.last-model-config-id",
			organization2ModelConfig.id,
		);
		mockPermittedOrganizations(
			{
				[MockDefaultOrganization.id]: false,
				[MockOrganization2.id]: true,
			},
			100,
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByRole("combobox", { name: "GPT 4.1 Mini" }),
		).toBeVisible();
	},
};

export const RestrictedMultiOrganizationUser: Story = {
	parameters: {
		showOrganizations: true,
		organizations: [MockDefaultOrganization, MockOrganization2],
		queries: [
			{
				key: organizationChatModelsKey(MockOrganization2.id),
				data: organization2LocalCatalog,
			},
			{
				key: userChatProviderConfigsKey,
				data: defaultUserProviderConfigs,
			},
		],
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
		queries: [
			{
				key: organizationChatModelsKey(MockOrganization2.id),
				data: organization2LocalCatalog,
			},
			{
				key: userChatProviderConfigsKey,
				data: defaultUserProviderConfigs,
			},
		],
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
		queries: [],
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
		spyOn(API.experimental, "getChatModels").mockResolvedValue(
			defaultModelCatalog,
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findAllByText(/authorization check failed/i);
		expect(API.experimental.getChatModels).not.toHaveBeenCalled();
		expect(API.experimental.getMCPServerConfigs).not.toHaveBeenCalled();
		expect(
			API.experimental.getUserChatPersonalModelOverrides,
		).not.toHaveBeenCalled();
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
		queries: [],
	},
	beforeEach: () => {
		localStorage.setItem(emptyInputStorageKey, "draft message");
		pendingOrganizationAuthorization = createDeferred();
		spyOn(API, "getOrganizations").mockResolvedValue([
			MockDefaultOrganization,
			MockOrganization2,
		]);
		spyOn(API, "checkAuthorization").mockImplementation(
			() => pendingOrganizationAuthorization.promise,
		);
		spyOn(API.experimental, "getChatModels").mockImplementation(
			async (organizationId) =>
				organizationId === MockOrganization2.id
					? organization2LocalCatalog
					: { ...defaultModelCatalog, models: [] },
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
		expect(API.experimental.getChatModels).not.toHaveBeenCalled();
		expect(API.experimental.getMCPServerConfigs).not.toHaveBeenCalled();
		expect(
			API.experimental.getUserChatPersonalModelOverrides,
		).not.toHaveBeenCalled();
		expect(
			canvas.queryByText(/AI models aren't available yet/i),
		).not.toBeInTheDocument();
		expect(canvas.queryByText("No model is available")).not.toBeInTheDocument();
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
		expect(
			canvas.queryByRole("button", { name: /organization/i }),
		).not.toBeInTheDocument();

		pendingOrganizationAuthorization.resolve({
			[MockDefaultOrganization.id]: false,
			[MockOrganization2.id]: true,
		});

		await waitFor(() => expect(sendButton).toBeEnabled());
		expect(API.experimental.getChatModels).toHaveBeenCalledWith(
			MockOrganization2.id,
		);
		expect(API.experimental.getChatModels).not.toHaveBeenCalledWith(
			MockDefaultOrganization.id,
		);
		expect(API.experimental.getMCPServerConfigs).toHaveBeenCalledWith(
			MockOrganization2.id,
		);
		expect(
			API.experimental.getUserChatPersonalModelOverrides,
		).toHaveBeenCalledWith(MockOrganization2.id, "me");
		// Positive control: once settled the same drop is accepted and
		// attaches, so the pending-state assertions exercised a real path.
		expect(dropFile("after.txt")).toBe(false);
		await waitFor(() =>
			expect(canvas.getByLabelText("Remove after.txt")).toBeInTheDocument(),
		);
	},
};

export const SelectedOrganizationSurvivesRemount: Story = {
	parameters: {
		showOrganizations: true,
		organizations: [MockDefaultOrganization, MockOrganization2],
		queries: [],
	},
	args: { ...defaultArgs },
	render: (args) => <RemountAgentCreateForm {...args} />,
	beforeEach: () => {
		mockPermittedOrganizations({
			[MockDefaultOrganization.id]: true,
			[MockOrganization2.id]: true,
		});
		spyOn(API.experimental, "getChatModels").mockImplementation(
			async (organizationId) =>
				organizationId === MockOrganization2.id
					? organization2LocalCatalog
					: defaultModelCatalog,
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", {
				name: `Organization: ${MockDefaultOrganization.display_name}`,
			}),
		);
		await userEvent.click(
			await screen.findByRole("option", {
				name: MockOrganization2.display_name,
			}),
		);
		await waitFor(() => {
			expect(localStorage.getItem(selectedOrganizationIdStorageKey)).toBe(
				MockOrganization2.id,
			);
		});

		await userEvent.click(canvas.getByRole("button", { name: "Remount form" }));
		expect(
			await canvas.findByRole("button", {
				name: `Organization: ${MockOrganization2.display_name}`,
			}),
		).toBeVisible();
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
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: defaultModelCatalog,
			},
			{
				key: userChatProviderConfigsKey,
				data: defaultUserProviderConfigs,
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

export const LocalOrganizationModels: Story = {
	parameters: {
		showOrganizations: true,
		organizations: [MockOrganization2],
		queries: [
			{
				key: permittedOrgsKey,
				data: [MockOrganization2],
			},
			{
				key: organizationChatModelsKey(MockOrganization2.id),
				data: organization2LocalCatalog,
			},
			{
				key: userChatProviderConfigsKey,
				data: defaultUserProviderConfigs,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.queryByRole("button", {
				name: `Organization: ${MockOrganization2.display_name}`,
			}),
		).not.toBeInTheDocument();
		expect(
			canvas.getByRole("combobox", { name: "GPT 4.1 Mini" }),
		).toBeVisible();
	},
};

export const ForeignOnlyModelsDisableGeneration: Story = {
	parameters: {
		showOrganizations: true,
		organizations: [MockOrganization2],
		queries: [
			{
				key: permittedOrgsKey,
				data: [MockOrganization2],
			},
			{
				key: organizationChatModelsKey(MockOrganization2.id),
				data: organization2ForeignOnlyCatalog,
			},
			{
				key: userChatProviderConfigsKey,
				data: defaultUserProviderConfigs,
			},
		],
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/AI models aren't available yet/)).toBeVisible();
		expect(
			canvas.getByRole("textbox", { name: "Chat message" }),
		).toHaveAttribute("aria-disabled", "true");
		expect(canvas.getByRole("button", { name: "Send" })).toBeDisabled();
		expect(args.onCreateChat).not.toHaveBeenCalled();
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
		queries: [
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: defaultModelCatalog,
			},
			{
				key: userChatProviderConfigsKey,
				data: defaultUserProviderConfigs,
			},
			{
				key: organizationChatModelsKey(MockOrganization2.id),
				data: organization2LocalCatalog,
			},
		],
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

export const MCPServersLoadingDisablesSend: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getMCPServerConfigs").mockImplementation(
			() => new Promise(() => {}),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByRole("textbox");
		await userEvent.click(input);
		await userEvent.keyboard("send while MCP servers load");
		expect(canvas.getByRole("button", { name: "Send" })).toBeDisabled();
	},
};

export const MCPServersErrorShowsAlertAndDisablesSend: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getMCPServerConfigs").mockRejectedValue(
			new Error("failed to load MCP servers"),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const matches = await canvas.findAllByText(/failed to load mcp servers/i);
		expect(matches.length).toBeGreaterThan(0);
		expect(canvas.getByRole("button", { name: "Send" })).toBeDisabled();
	},
};

export const MCPServersRefetchErrorKeepsSendEnabled: Story = {
	decorators: [
		(Story) => {
			capturedQueryClient = useQueryClient();
			return <Story />;
		},
	],
	beforeEach: () => {
		spyOn(API.experimental, "getMCPServerConfigs")
			.mockResolvedValueOnce([])
			.mockRejectedValue(new Error("failed to refresh MCP servers"));
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByRole("textbox");
		await userEvent.click(input);
		await userEvent.keyboard("send after a failed refetch");
		const send = canvas.getByRole("button", { name: "Send" });
		await waitFor(() => expect(send).toBeEnabled());
		if (!capturedQueryClient) {
			throw new Error("query client was not captured by the story decorator");
		}
		await capturedQueryClient.refetchQueries();
		expect(
			canvas.queryByText(/failed to refresh mcp servers/i),
		).not.toBeInTheDocument();
		expect(send).toBeEnabled();
	},
};
