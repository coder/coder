import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import {
	mockClaude,
	mockDisabledModel,
	mockGPT5,
} from "#/pages/AISettingsPage/ModelsPage/testFixtures";
import { ModelRow } from "./ModelRow";

const meta: Meta<typeof ModelRow> = {
	title: "pages/AISettingsPage/ModelsPage/ModelRow",
	component: ModelRow,
	args: {
		model: mockGPT5,
		providerLabel: "OpenAI",
	},
	decorators: [
		(Story) => (
			<Table className="table-fixed" aria-label="Models">
				<TableHeader>
					<TableRow>
						<TableHead className="w-1/3">Name</TableHead>
						<TableHead className="w-1/4">Provider</TableHead>
						<TableHead className="w-1/4">Context limit</TableHead>
						<TableHead className="w-40">Status</TableHead>
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
type Story = StoryObj<typeof ModelRow>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("GPT-5")).toBeInTheDocument();
		await expect(canvas.getByText("OpenAI")).toBeInTheDocument();
		await expect(canvas.getByText("200,000 tokens")).toBeInTheDocument();
		await expect(canvas.getByText("Enabled")).toBeInTheDocument();
		await expect(canvas.getByText("Default")).toBeInTheDocument();
	},
};

export const Enabled: Story = {
	args: {
		model: mockClaude,
		providerLabel: "Anthropic",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Enabled")).toBeInTheDocument();
		await expect(canvas.getByText("Anthropic")).toBeInTheDocument();
		await expect(canvas.queryByText("Default")).not.toBeInTheDocument();
	},
};

export const Disabled: Story = {
	args: {
		model: mockDisabledModel,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Disabled")).toBeInTheDocument();
		await expect(canvas.queryByText("Default")).not.toBeInTheDocument();
		await expect(canvas.getByText("128,000 tokens")).toBeInTheDocument();
	},
};

export const FallbackToModelName: Story = {
	args: {
		model: { ...mockGPT5, display_name: "" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("gpt-5")).toBeInTheDocument();
	},
};
