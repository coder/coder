import type { Meta, StoryObj } from "@storybook/react-vite";
import { SummaryPanel } from "./SummaryPanel";

const meta: Meta<typeof SummaryPanel> = {
	title: "pages/AgentsPage/SummaryPanel",
	component: SummaryPanel,
	decorators: [
		(Story) => (
			<div className="h-96 w-80 border border-border-default">
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof SummaryPanel>;

export const Default: Story = {
	args: {
		chatTitle: undefined,
	},
};

export const WithTitle: Story = {
	args: {
		chatTitle: "Fix login bug",
	},
};
