import type { Meta, StoryObj } from "@storybook/react";
import { XGrid } from "./XGrid";

const meta: Meta<typeof XGrid> = {
	title: "components/GanttChart/XGrid",
	component: XGrid,
	args: {
		columnWidth: 130,
		columns: 10,
	},
	decorators: [
		(Story) => (
			<div style={{ width: "1050", height: 500 }}>
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof XGrid>;

export const Default: Story = {};
