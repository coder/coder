import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChatModel } from "#/testHelpers/chatModels";
import DefaultsPageView from "./DefaultsPageView";

const model: TypesGen.ChatModel = {
	...MockChatModel,
	id: "model-1",
	display_name: "Model One",
};
const overrides: readonly TypesGen.ChatModelOverrideResponse[] = [
	{ context: "general", model_config_id: "model-1", reasoning_effort: "high" },
	{ context: "compaction", model_config_id: "model-1" },
];
const saveByContext = new Map<
	TypesGen.ChatModelOverrideContext,
	(req: TypesGen.UpdateChatModelOverrideRequest) => void
>([
	["general", fn()],
	["explore", fn()],
	["title_generation", fn()],
	["compaction", fn()],
	["advisor", fn()],
]);

const meta: Meta<typeof DefaultsPageView> = {
	title: "pages/AISettingsPage/ModelsPage/DefaultsPageView",
	component: DefaultsPageView,
	args: {
		overrides,
		enabledModels: [model],
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
type Story = StoryObj<typeof DefaultsPageView>;

export const SetAndUnset: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("General subagent")).toBeVisible();
		await expect(canvas.getAllByText("Use default").length).toBeGreaterThan(0);
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
export const NoModels: Story = {
	args: { enabledModels: [] },
	play: async ({ canvasElement }) => {
		await expect(within(canvasElement).getByRole("status")).toHaveTextContent(
			"no enabled chat models",
		);
	},
};
