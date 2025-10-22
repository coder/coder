import type { Meta, StoryObj } from "@storybook/react-vite";
import { Select, SelectContent, SelectItem } from "components/Select/Select";
import { userEvent, within } from "storybook/test";
import { PromptSelectTrigger } from "./PromptSelectTrigger";

const meta: Meta<typeof PromptSelectTrigger> = {
	title: "modules/tasks/TaskPrompt/PromptSelectTrigger",
	component: PromptSelectTrigger,
	args: {
		children: "Select a version",
		tooltip: "Template version",
	},
	render: (args) => (
		<Select>
			<PromptSelectTrigger {...args} />
			<SelectContent>
				<SelectItem value="version-1">Version 1</SelectItem>
				<SelectItem value="version-2">Version 2</SelectItem>
				<SelectItem value="version-3">Version 3</SelectItem>
			</SelectContent>
		</Select>
	),
};

export default meta;
type Story = StoryObj<typeof PromptSelectTrigger>;

export const Closed: Story = {};

export const Open: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("combobox");
		await userEvent.click(trigger);
	},
};
