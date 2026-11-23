import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import type { UnpricedAIModel } from "#/api/typesGenerated";
import {
	MockUnpricedAIModel,
	MockUnpricedAIModels,
} from "#/testHelpers/entities";
import { UnpricedModelsPopover } from "./UnpricedModelsPopover";

// The first name is long enough to truncate.
const mockManyModels: UnpricedAIModel[] = Array.from(
	{ length: 12 },
	(_, i) => ({
		...MockUnpricedAIModel,
		model:
			i === 0
				? "us.anthropic.claude-opus-4-8-with-a-long-inference-profile-v1:0"
				: `claude-opus-${i + 1}`,
	}),
);

const meta = {
	title: "pages/AISettingsPage/AISpendPage/UnpricedModelsPopover",
	component: UnpricedModelsPopover,
	args: { models: MockUnpricedAIModels.models },
	play: async () => {
		await userEvent.click(
			screen.getByRole("button", { name: /Models? not set up/ }),
		);
	},
} satisfies Meta<typeof UnpricedModelsPopover>;

export default meta;
type Story = StoryObj<typeof UnpricedModelsPopover>;

export const Closed: Story = {
	play: async () => {},
};

export const Open: Story = {};

export const SingleModel: Story = {
	args: { models: [MockUnpricedAIModel] },
};

/** The list keeps its height and scrolls. */
export const ManyModels: Story = {
	args: { models: mockManyModels },
};

export const ProviderWithoutIcon: Story = {
	args: {
		models: [
			{
				...MockUnpricedAIModel,
				provider_name: "arcee",
				provider_display_name: "Arcee",
				provider_type: "openai-compat",
				model: "arcee-ai/trinity-large-thinking",
			},
		],
	},
};

export const ProviderWithCustomIcon: Story = {
	args: {
		models: [
			{
				...MockUnpricedAIModel,
				provider_name: "openrouter",
				provider_display_name: "OpenRouter",
				provider_icon: "/icon/openrouter.svg",
				provider_type: "openai-compat",
				model: "moonshotai/kimi-k2",
			},
		],
	},
};

/** The same model is unpriced on two providers of the same type. */
export const SameModelOnTwoProviders: Story = {
	args: {
		models: [
			MockUnpricedAIModel,
			{ ...MockUnpricedAIModel, provider_name: "anthropic-eu" },
		],
	},
};
