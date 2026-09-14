import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	AgentChatPageSkeleton,
	AgentsPageLayoutSkeleton,
} from "./AgentsSkeletons";

const meta: Meta<typeof AgentsPageLayoutSkeleton> = {
	title: "pages/AgentsPage/AgentsSkeletons",
	component: AgentsPageLayoutSkeleton,
	decorators: [
		(Story) => (
			<div style={{ height: 600, width: "100%" }}>
				<Story />
			</div>
		),
	],
};
export default meta;
type Story = StoryObj<typeof AgentsPageLayoutSkeleton>;

export const Page: Story = {};

export const Detail: Story = {
	render: () => (
		<div style={{ height: 600, width: "100%" }}>
			<AgentChatPageSkeleton />
		</div>
	),
};
