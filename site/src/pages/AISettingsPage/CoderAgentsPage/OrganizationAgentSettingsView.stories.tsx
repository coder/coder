import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChatModel } from "#/testHelpers/chatModels";
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
		overrides,
		enabledModels: [model, alternateModel],
		providerInfoByID: new Map([
			[
				model.ai_provider_id,
				{ provider: "openai", displayName: "OpenAI", icon: "" },
			],
		]),
		isLoading: false,
		loadError: null,
		refetchError: null,
		canEdit: true,
		showAdvisor: true,
		saveByContext,
		savingContexts: new Set(),
		errorContexts: new Set(),
	},
};
export default meta;
type Story = StoryObj<typeof OrganizationAgentSettingsView>;

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
			within(exploreSection).getByRole("combobox", { name: "Use default" }),
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
export const UnavailableSavedModels: Story = {
	args: {
		overrides: [
			{ context: "general", model_config_id: "model-gone" },
			{ context: "title_generation", model_config_id: "model-gone" },
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Title generation fails hard on a broken override, so its warning must
		// not claim the model is ignored in favor of default selection.
		const titleSection = canvas.getByRole("form", {
			name: "Title generation",
		});
		await expect(
			within(titleSection).getByText(/Title generation will be skipped/),
		).toBeVisible();
		const generalSection = canvas.getByRole("form", {
			name: "General subagent",
		});
		await expect(
			within(generalSection).getByText(/will be ignored/),
		).toBeVisible();
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
