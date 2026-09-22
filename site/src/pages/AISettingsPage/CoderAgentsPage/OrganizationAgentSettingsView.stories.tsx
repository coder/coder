import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChatModel } from "#/testHelpers/chatModels";
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
		isLoading: false,
		isOverridesLoading: false,
		loadError: null,
		refetchError: null,
		modelsError: null,
		canEdit: true,
		showAdvisor: true,
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
export const AdvisorDisabled: Story = {
	args: { showAdvisor: false },
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).queryByText("Advisor"),
		).not.toBeInTheDocument();
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
		isLoading: true,
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
		refetchError: mockApiError({
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
		loadError: mockApiError({ message: "Failed to load model overrides." }),
	},
};

export const NoModelsWithOverridesRefetchError: Story = {
	args: {
		enabledModels: [],
		refetchError: mockApiError({
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
