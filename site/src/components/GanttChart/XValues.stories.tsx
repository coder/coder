import type { Meta, StoryObj } from "@storybook/react";
import { XValues } from "./XValues";

const meta: Meta<typeof XValues> = {
	title: "components/GanttChart/XValues",
	component: XValues,
	args: {
		columnWidth: 130,
		values: [
			"00:00:05",
			"00:00:10",
			"00:00:15",
			"00:00:20",
			"00:00:25",
			"00:00:30",
			"00:00:35",
			"00:00:40",
			"00:00:45",
		],
	},
};

export default meta;
type Story = StoryObj<typeof XValues>;

export const Default: Story = {};
