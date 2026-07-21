import type { Meta, StoryObj } from "@storybook/react-vite";
import { AgentChatPageSkeleton, AgentsLayoutSkeleton } from "./AgentsSkeletons";

const meta: Meta<typeof AgentsLayoutSkeleton> = {
	title: "pages/AgentsPage/AgentsSkeletons",
	component: AgentsLayoutSkeleton,
	decorators: [
		(Story) => (
			<div style={{ height: 600, width: "100%" }}>
				<Story />
			</div>
		),
	],
};
export default meta;
type Story = StoryObj<typeof AgentsLayoutSkeleton>;

export const Page: Story = {};

export const Detail: Story = {
	render: () => (
		<div style={{ height: 600, width: "100%" }}>
			<AgentChatPageSkeleton />
		</div>
	),
};
