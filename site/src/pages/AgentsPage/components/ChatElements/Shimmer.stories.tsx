import type { Meta, StoryObj } from "@storybook/react-vite";
import { Shimmer } from "./Shimmer";

const meta: Meta<typeof Shimmer> = {
	title: "pages/AgentsPage/ChatElements/Shimmer",
	component: Shimmer,
	args: {
		children: "Thinking",
		as: "span",
		className: "text-[13px] leading-6",
	},
};

export default meta;
type Story = StoryObj<typeof Shimmer>;

export const Default: Story = {};

export const Labels: Story = {
	render: (args) => (
		<div className="flex w-80 max-w-full flex-col items-start gap-4 p-4">
			<Shimmer {...args}>Thinking</Shimmer>
			<Shimmer {...args}>Interrupting</Shimmer>
			<Shimmer {...args}>Reading workspace files</Shimmer>
			<Shimmer {...args} className="max-w-full truncate text-[13px] leading-6">
				Searching the repository for references to the workspace configuration
			</Shimmer>
			<Shimmer {...args}>正在检查工作区配置</Shimmer>
			<Shimmer {...args}>A</Shimmer>
			<Shimmer {...args} className="text-lg">
				Generating response
			</Shimmer>
		</div>
	),
};

export const Light: Story = {
	...Labels,
	parameters: { themes: { themeOverride: "light" } },
};

export const CustomDurationAndSpread: Story = {
	args: { children: "Reading workspace files", duration: 4, spread: 1 },
};
