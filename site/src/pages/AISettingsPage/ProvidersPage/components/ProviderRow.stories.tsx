import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import {
	MockAIProviderAnthropic,
	MockAIProviderBedrock,
	MockAIProviderCopilot,
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
			<Table className="table-fixed" aria-label="AI providers">
				<TableHeader>
					<TableRow>
						<TableHead className="w-[42%]">Name</TableHead>
						<TableHead className="w-[38%]">Base URL</TableHead>
						<TableHead className="w-20 text-center">
							<span className="sr-only">Enabled</span>
						</TableHead>
						<TableHead className="w-12">
							<span className="sr-only">Open provider</span>
						</TableHead>
					</TableRow>
				</TableHeader>
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

export const LongText: Story = {
	args: {
		provider: {
			...MockAIProviderBedrock,
			name: "bedrock12341234bedrock12341234bedrock12341234",
			display_name: "thisisacoolexample11",
			base_url:
				"https://bedrock-runtime.us-east-2.amazonaws.com/very/long/path/segment",
		},
	},
};

// Copilot is unsupported by Agents, so the row shows the label.
export const NotSupportedInAgents: Story = {
	args: {
		provider: MockAIProviderCopilot,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("Not supported in Agents"),
		).toBeInTheDocument();
	},
};

export const SupportedHasNoAgentsLabel: Story = {
	args: {
		provider: { ...MockAIProviderOpenAI, enabled: true },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("OpenAI")).toBeInTheDocument();
		await expect(
			canvas.queryByText("Not supported in Agents"),
		).not.toBeInTheDocument();
	},
};
