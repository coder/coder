import type { Meta, StoryObj } from "@storybook/react-vite";
import { AgentChatPageSkeleton, AgentsPageSkeleton } from "./AgentsSkeletons";

const meta: Meta<typeof AgentsPageSkeleton> = {
	title: "pages/AgentsPage/AgentsSkeletons",
	component: AgentsPageSkeleton,
	decorators: [
		(Story) => (
			<div style={{ height: 600, width: "100%" }}>
				<Story />
			</div>
		),
	],
};
export default meta;
type Story = StoryObj<typeof AgentsPageSkeleton>;

export const Page: Story = {};

export const Detail: Story = {
	render: () => (
		<div style={{ height: 600, width: "100%" }}>
			<AgentChatPageSkeleton />
		</div>
	),
};
