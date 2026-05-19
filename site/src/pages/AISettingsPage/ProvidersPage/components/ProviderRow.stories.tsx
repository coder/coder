import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { Table, TableBody } from "#/components/Table/Table";
import {
	MockAIProviderAnthropic,
	MockAIProviderBedrock,
	MockAIProviderOpenAI,
} from "#/testHelpers/entities";
import { ProviderRow } from "./ProviderRow";

const meta: Meta<typeof ProviderRow> = {
	title: "pages/AISettingsPage/ProviderRow",
	component: ProviderRow,
	args: {
		onClick: fn(),
	},
	decorators: [
		(Story) => (
			<Table>
				<TableBody>
					<Story />
				</TableBody>
			</Table>
		),
	],
};

export default meta;
type Story = StoryObj<typeof ProviderRow>;

export const OpenAI: Story = {
	args: {
		provider: MockAIProviderOpenAI,
	},
};

export const Anthropic: Story = {
	args: {
		provider: MockAIProviderAnthropic,
	},
};

export const Bedrock: Story = {
	args: {
		provider: MockAIProviderBedrock,
	},
};
