import type { Meta, StoryObj } from "@storybook/react-vite";
import { Supergraphic } from "./Supergraphic";

const meta: Meta<typeof Supergraphic> = {
	title: "components/Supergraphic",
	component: Supergraphic,
	decorators: [
		(Story) => (
			<div className="relative h-64 w-96 rounded-lg border border-solid border-border-default overflow-hidden">
				<Story />
			</div>
		),
	],
	args: {},
};

export default meta;
type Story = StoryObj<typeof Supergraphic>;

export const Default: Story = {};

export const CustomFaded: Story = {
	args: {
		className:
			"opacity-40 [mask-image:linear-gradient(to_right,transparent,black_40%)]",
	},
};
