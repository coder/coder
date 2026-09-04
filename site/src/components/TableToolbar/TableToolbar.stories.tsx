import type { Meta, StoryObj } from "@storybook/react-vite";
import { TableToolbar } from "./TableToolbar";

const meta: Meta<typeof TableToolbar> = {
	title: "components/TableToolbar",
	component: TableToolbar,
};

export default meta;
type Story = StoryObj<typeof TableToolbar>;

export const Default: Story = {
	args: {
		children: (
			<div>
				Showing <strong>10</strong> of <strong>100</strong> items
			</div>
		),
	},
};
