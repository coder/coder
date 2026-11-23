import type { Meta, StoryObj } from "@storybook/react-vite";
import { MockUnpricedAIModels } from "#/testHelpers/entities";
import { TotalSpendCard } from "./TotalSpendCard";

const meta = {
	title: "pages/AISettingsPage/AISpendPage/TotalSpendCard",
	component: TotalSpendCard,
	args: {
		spendMicros: 31_188_990_000,
		unpricedModels: MockUnpricedAIModels.models,
		isUnpricedModelsLoading: false,
	},
	decorators: [
		(Story) => (
			<div className="max-w-xl">
				<Story />
			</div>
		),
	],
} satisfies Meta<typeof TotalSpendCard>;

export default meta;
type Story = StoryObj<typeof TotalSpendCard>;

export const UnpricedModels: Story = {};

export const SingleUnpricedModel: Story = {
	args: { unpricedModels: MockUnpricedAIModels.models.slice(0, 1) },
};

export const AllModelsPriced: Story = {
	args: { unpricedModels: [] },
};

/** The total arrives before the slower unpriced models check. */
export const UnpricedModelsLoading: Story = {
	args: { unpricedModels: undefined, isUnpricedModelsLoading: true },
};

/** The unpriced models request failed, for example with a 403. */
export const UnpricedModelsUnavailable: Story = {
	args: { unpricedModels: undefined },
};

export const Loading: Story = {
	args: {
		spendMicros: undefined,
		unpricedModels: undefined,
		isUnpricedModelsLoading: true,
	},
};

export const NoSpend: Story = {
	args: { spendMicros: 0, unpricedModels: [] },
};
