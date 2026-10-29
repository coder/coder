import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChatModel,
	MockCompactionChatModel,
	MockComputerUseChatModel,
} from "#/testHelpers/chatModels";
import { mockApiError } from "#/testHelpers/entities";
import OrganizationAgentSettingsView from "./OrganizationAgentSettingsView";

const model: TypesGen.ChatModel = {
	...MockChatModel,
	id: "model-1",
	display_name: "Model One",
};
const alternateModel: TypesGen.ChatModel = {
	...MockChatModel,
	id: "model-2",
	model: "model-two",
	display_name: "Model Two",
};
const mockGoogleModel: TypesGen.ChatModel = {
	...MockChatModel,
	id: "model-gemini",
	ai_provider_id: "provider-google",
	model: "gemini-3.8-flash",
	display_name: "Gemini Flash",
};
const mockTinyModelWithCompactionOff: TypesGen.ChatModel = {
	...MockChatModel,
	id: "tiny-model",
	model: "tiny",
	display_name: "Tiny Model",
	context_limit: 8_000,
	compression_threshold: 100,
};
const mockWarningModels = [
	MockCompactionChatModel,
	model,
	{ ...alternateModel, compression_threshold: 100 },
	mockTinyModelWithCompactionOff,
];
const mockWarningOverrides: readonly TypesGen.ChatModelOverrideResponse[] = [
	{ context: "compaction", model_config_id: model.id },
];
const saveGeneralOverride = fn();
const saveExploreOverride = fn();
const overrides: readonly TypesGen.ChatModelOverrideResponse[] = [
	{ context: "general", model_config_id: "model-1", reasoning_effort: "high" },
	{ context: "compaction", model_config_id: "model-1" },
];
const saveByContext = new Map<
	TypesGen.ChatModelOverrideContext,
	(req: TypesGen.UpdateChatModelOverrideRequest) => void
>([
	["general", saveGeneralOverride],
	["explore", saveExploreOverride],
	["title_generation", fn()],
	["compaction", fn()],
	["advisor", fn()],
	["computer_use", fn()],
]);

const meta: Meta<typeof OrganizationAgentSettingsView> = {
	title: "pages/AISettingsPage/CoderAgentsPage/OrganizationAgentSettingsView",
	component: OrganizationAgentSettingsView,
	args: {
		defaultModelID: model.id,
		onSaveDefaultModel: fn(),
		isSavingDefaultModel: false,
		isSaveDefaultModelError: false,
		overrides,
		enabledModels: [model, alternateModel],
		providerInfoByID: new Map([
			[
				model.ai_provider_id,
				{ provider: "openai", displayName: "OpenAI", icon: "" },
			],
		]),
		isModelsLoading: false,
		isOverridesLoading: false,
		overridesLoadError: null,
		overridesRefetchError: null,
		modelsError: null,
		canEdit: true,
		showAdvisor: true,
		showComputerUse: false,
		saveByContext,
		savingContexts: new Set(),
		errorContexts: new Set(),
	},
};
export default meta;
type Story = StoryObj<typeof OrganizationAgentSettingsView>;

export const DefaultModelOpen: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("combobox", {
				name: /^Default model,/,
			}),
		);
	},
};

export const UnavailableDefaultModel: Story = {
	args: { defaultModelID: "model-gone" },
};

export const NoDefaultModel: Story = {
	args: { defaultModelID: undefined },
};

export const ModelsLoadError: Story = {
	args: {
		defaultModelID: undefined,
		enabledModels: [],
		modelsError: mockApiError({ message: "Failed to load chat models." }),
	},
};

export const SavingDefaultModel: Story = {
	args: { isSavingDefaultModel: true },
};

export const DefaultModelSaveError: Story = {
	args: { isSaveDefaultModelError: true },
	// The error follows a failed save of a new pick, so the row is dirty.
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("combobox", {
				name: /^Default model,/,
			}),
		);
		await userEvent.click(
			await screen.findByRole("option", {
				name: new RegExp(alternateModel.display_name),
			}),
		);
	},
};

export const SetAndUnset: Story = {
	beforeEach: () => {
		saveGeneralOverride.mockClear();
		saveExploreOverride.mockClear();
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const generalSection = canvas.getByRole("form", {
			name: "General subagent",
		});
		const exploreSection = canvas.getByRole("form", {
			name: "Explore subagent",
		});

		await userEvent.click(
			within(exploreSection).getByRole("combobox", {
				name: "Explore subagent, Use chat model",
			}),
		);
		await userEvent.click(
			await body.findByRole("option", { name: /Model Two/i }),
		);
		const exploreSave = within(exploreSection).getByRole("button", {
			name: "Save",
		});
		await waitFor(() => expect(exploreSave).toBeEnabled());
		await userEvent.click(exploreSave);
		await waitFor(() => {
			expect(saveExploreOverride).toHaveBeenCalledWith(
				{ model_config_id: alternateModel.id },
				expect.anything(),
			);
		});

		await userEvent.click(
			within(generalSection).getByRole("button", { name: "Clear" }),
		);
		const generalSave = within(generalSection).getByRole("button", {
			name: "Save",
		});
		await waitFor(() => expect(generalSave).toBeEnabled());
		await userEvent.click(generalSave);
		await waitFor(() => {
			expect(saveGeneralOverride).toHaveBeenCalledWith(
				{ model_config_id: "" },
				expect.anything(),
			);
		});
	},
};

export const CompactionTriggerWarning: Story = {
	args: {
		overrides: mockWarningOverrides,
		enabledModels: mockWarningModels,
		// Unavailable to this viewer; the admin alert must still warn because it
		// ignores per-user provider access.
		providerInfoByID: new Map([
			[
				model.ai_provider_id,
				{
					provider: "openai",
					displayName: "OpenAI",
					icon: "",
					available: false,
				},
			],
		]),
	},
	play: async ({ canvasElement }) => {
		const section = within(
			within(canvasElement).getByRole("form", { name: "Compaction" }),
		);

		await userEvent.click(
			section.getByRole("combobox", { name: /Model One/i }),
		);
		await userEvent.click(
			await screen.findByRole("option", { name: /Compact Mini/i }),
		);
	},
};

export const CompactionTriggerWarningWithoutCompactionOff: Story = {
	args: {
		overrides: mockWarningOverrides,
		enabledModels: [MockCompactionChatModel, model, alternateModel],
	},
	play: CompactionTriggerWarning.play,
};

export const CompactionTriggerWarningManyModels: Story = {
	args: {
		overrides: mockWarningOverrides,
		enabledModels: [
			MockCompactionChatModel,
			model,
			alternateModel,
			{ ...MockChatModel, id: "model-3", display_name: "Model Three" },
			{ ...MockChatModel, id: "model-4", display_name: "Model Four" },
			{ ...MockChatModel, id: "model-5", display_name: "Model Five" },
		],
	},
	play: CompactionTriggerWarning.play,
};

export const AdvisorDisabled: Story = {
	args: { showAdvisor: false },
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).queryByText("Advisor"),
		).not.toBeInTheDocument();
	},
};

const computerUseArgs = {
	showComputerUse: true,
	enabledModels: [
		model,
		alternateModel,
		MockComputerUseChatModel,
		mockGoogleModel,
	],
	providerInfoByID: new Map([
		[
			model.ai_provider_id,
			{ provider: "openai", displayName: "OpenAI", icon: "" },
		],
		[
			MockComputerUseChatModel.ai_provider_id,
			{ provider: "anthropic", displayName: "Anthropic", icon: "" },
		],
		[
			mockGoogleModel.ai_provider_id,
			{ provider: "google", displayName: "Google", icon: "" },
		],
	]),
};
const mockUnverifiedComputerUseOverrides: readonly TypesGen.ChatModelOverrideResponse[] =
	[...overrides, { context: "computer_use", model_config_id: model.id }];

// The gpt-5 model behind model-1 is not on the curated computer use list.
export const ComputerUseUnverifiedModel: Story = {
	args: { ...computerUseArgs, overrides: mockUnverifiedComputerUseOverrides },
	play: async ({ canvasElement }) => {
		within(canvasElement)
			.getByRole("button", { name: "Computer use model warning" })
			.focus();
	},
};

export const ComputerUseVerifiedModel: Story = {
	args: {
		...computerUseArgs,
		overrides: [
			...overrides,
			{ context: "computer_use", model_config_id: MockComputerUseChatModel.id },
		],
	},
};

export const ComputerUseUnverifiedDefaultModel: Story = {
	args: computerUseArgs,
	play: ComputerUseUnverifiedModel.play,
};

const mockResponsesAPIDisabledModel: TypesGen.ChatModel = {
	...MockChatModel,
	id: "model-gpt-5-4",
	model: "gpt-5.4",
	display_name: "GPT-5.4",
	model_config: { openai_config: { use_responses_api: false } },
};

// gpt-5.4 is on the curated list, but OpenAI computer use needs the Responses
// API.
export const ComputerUseResponsesAPIDisabled: Story = {
	args: {
		...computerUseArgs,
		enabledModels: [
			...computerUseArgs.enabledModels,
			mockResponsesAPIDisabledModel,
		],
		overrides: [
			...overrides,
			{
				context: "computer_use",
				model_config_id: mockResponsesAPIDisabledModel.id,
			},
		],
	},
	play: ComputerUseUnverifiedModel.play,
};

export const ComputerUseNoDefaultModel: Story = {
	args: { ...computerUseArgs, defaultModelID: undefined },
};

export const ComputerUseModelOptions: Story = {
	args: computerUseArgs,
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("combobox", { name: /^Computer use,/ }),
		);
	},
};

export const ComputerUseNoProviderModels: Story = {
	args: { ...computerUseArgs, enabledModels: [mockGoogleModel] },
};

export const ComputerUseUnavailableSavedModel: Story = {
	args: {
		...computerUseArgs,
		overrides: [
			...overrides,
			{ context: "computer_use", model_config_id: "model-gone" },
		],
	},
};

export const ComputerUseOverridesLoading: Story = {
	args: { ...computerUseArgs, isOverridesLoading: true, overrides: undefined },
};

export const ComputerUseSaveError: Story = {
	args: { ...computerUseArgs, errorContexts: new Set(["computer_use"]) },
};

export const ComputerUseReadOnly: Story = {
	args: {
		...computerUseArgs,
		canEdit: false,
		overrides: mockUnverifiedComputerUseOverrides,
	},
};

export const ReadOnly: Story = {
	args: { canEdit: false },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		for (const button of canvas.getAllByRole("button"))
			await expect(button).toBeDisabled();
	},
};
// The title generation section must show its own "skipped" warning and the
// general section the generic "ignored" one; the screenshot covers both.
export const UnavailableSavedModels: Story = {
	args: {
		overrides: [
			{ context: "general", model_config_id: "model-gone" },
			{ context: "title_generation", model_config_id: "model-gone" },
		],
	},
};

export const Loading: Story = {
	args: {
		isModelsLoading: true,
		isOverridesLoading: true,
		defaultModelID: undefined,
		overrides: undefined,
		enabledModels: [],
	},
};

export const OverridesLoading: Story = {
	args: {
		isOverridesLoading: true,
		overrides: undefined,
	},
};

export const OverridesRefetchError: Story = {
	args: {
		overridesRefetchError: mockApiError({
			message: "Failed to refresh model overrides.",
		}),
	},
};

export const ModelsRefetchError: Story = {
	args: {
		modelsError: mockApiError({ message: "Failed to refresh chat models." }),
	},
};

export const OverridesLoadError: Story = {
	args: {
		overrides: undefined,
		overridesLoadError: mockApiError({
			message: "Failed to load model overrides.",
		}),
	},
};

export const NoModelsWithOverridesRefetchError: Story = {
	args: {
		enabledModels: [],
		overridesRefetchError: mockApiError({
			message: "Failed to refresh model overrides.",
		}),
	},
};

export const NoModels: Story = {
	args: { enabledModels: [] },
	beforeEach: () => {
		saveGeneralOverride.mockClear();
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("status")).toHaveTextContent(
			"no enabled chat models",
		);
		// A saved override that references a now-disabled model must remain
		// clearable even though no replacement model can be selected.
		const generalSection = canvas.getByRole("form", {
			name: "General subagent",
		});
		const clear = within(generalSection).getByRole("button", {
			name: "Clear",
		});
		await expect(clear).toBeEnabled();
		await userEvent.click(clear);
		const save = within(generalSection).getByRole("button", { name: "Save" });
		await waitFor(() => expect(save).toBeEnabled());
		await userEvent.click(save);
		await waitFor(() => {
			expect(saveGeneralOverride).toHaveBeenCalledWith(
				{ model_config_id: "" },
				expect.anything(),
			);
		});
	},
};
