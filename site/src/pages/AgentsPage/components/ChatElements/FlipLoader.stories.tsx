import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { FlipLoader } from "./FlipLoader";

const meta: Meta<typeof FlipLoader> = {
	title: "pages/AgentsPage/ChatElements/FlipLoader",
	component: FlipLoader,
	args: { label: "Tool call running" },
};
export default meta;
type Story = StoryObj<typeof FlipLoader>;

export const IconSlot: Story = {
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).getByRole("img", { name: "Tool call running" }),
		).toBeInTheDocument();
	},
};

export const Sizes: Story = {
	render: (args) => (
		<div className="flex items-end gap-8 p-6">
			{[16, 32, 64, 128].map((size) => (
				<div key={size} className="flex flex-col items-center gap-2">
					<FlipLoader {...args} size={size} />
					<span className="text-xs text-content-secondary">{size}px</span>
				</div>
			))}
		</div>
	),
};

export const InToolRow: Story = {
	render: (args) => (
		<div className="flex items-center gap-2 text-[13px] leading-6 text-content-secondary">
			<FlipLoader {...args} />
			<span>Adding the streaming behavior to the working block</span>
		</div>
	),
};
