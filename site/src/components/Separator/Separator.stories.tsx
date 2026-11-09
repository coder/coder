import type { Meta, StoryObj } from "@storybook/react-vite";
import { Separator } from "./Separator";

const meta: Meta<typeof Separator> = {
	title: "components/Separator",
	component: Separator,
};

export default meta;
type Story = StoryObj<typeof Separator>;

export const Horizontal: Story = {
	decorators: [
		(Story) => (
			<div className="w-64">
				<Story />
			</div>
		),
	],
};

export const Vertical: Story = {
	args: { orientation: "vertical" },
	decorators: [
		(Story) => (
			<div className="h-16">
				<Story />
			</div>
		),
	],
};
