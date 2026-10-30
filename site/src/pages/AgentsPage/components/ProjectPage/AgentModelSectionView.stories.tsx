import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import { MockModelSelectorOption } from "#/modules/aiModels/modelSelectorFixtures";
import { mockApiError } from "#/testHelpers/entities";
import { AgentModelSectionView } from "./AgentModelSection";

const meta: Meta<typeof AgentModelSectionView> = {
	title: "pages/AgentsPage/ProjectPage/AgentModelSectionView",
	component: AgentModelSectionView,
	decorators: [
		(Story) => (
			<div className="max-w-3xl">
				<Story />
			</div>
		),
	],
	args: {
		options: [MockModelSelectorOption],
		defaultModelName: MockModelSelectorOption.displayName,
		modelConfigId: "",
		modelStatus: "default",
		modelStatusReason: undefined,
		modelDisplayName: undefined,
		isLoading: false,
		modelsError: undefined,
		onRetryModels: fn(),
		canEdit: true,
		isSaving: false,
		saveError: undefined,
		onChange: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof AgentModelSectionView>;

export const Default: Story = {};

export const UnavailableModel: Story = {
	args: {
		modelConfigId: "deleted-model",
		modelStatus: "unavailable",
		modelStatusReason: "The model was deleted.",
		modelDisplayName: "Claude Sonnet",
	},
};

export const SetModel: Story = {
	args: {
		modelConfigId: MockModelSelectorOption.id,
		modelStatus: "available",
		modelDisplayName: MockModelSelectorOption.displayName,
	},
};

export const ReadOnly: Story = {
	args: { ...SetModel.args, canEdit: false },
};

export const Saving: Story = { args: { ...SetModel.args, isSaving: true } };

export const Loading: Story = { args: { options: [], isLoading: true } };

export const ModelsError: Story = {
	args: {
		options: [],
		modelsError: mockApiError({ message: "Failed to load models." }),
	},
};

export const SaveError: Story = {
	args: {
		saveError: mockApiError({
			message: "Invalid request.",
			validations: [
				{
					field: "model_config_id",
					detail: "The model is not enabled.",
				},
			],
		}),
	},
};

export const PickerOpen: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			await within(canvasElement).findByRole("combobox", {
				name: /Agent model/,
			}),
		);
	},
};
