import type { Meta, StoryObj } from "@storybook/react-vite";
import { chatFullWidthStorageKey } from "../hooks/useChatFullWidth";
import { overviewCardOpenStorageKey } from "../utils/overviewCardStorage";
import {
	AgentChatPageSkeleton,
	AgentsPageLayoutSkeleton,
} from "./AgentsSkeletons";
import { EmbedContext } from "./EmbedContext";

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

const resetOverviewCardStorage = () => {
	const reset = () => {
		localStorage.removeItem(overviewCardOpenStorageKey);
		localStorage.removeItem(chatFullWidthStorageKey);
	};
	reset();
	return reset;
};

const OverviewCardReserveFrame: React.FC<React.PropsWithChildren> = ({
	children,
}) => (
	<div style={{ display: "flex", height: 600, width: 72 * 16 }}>{children}</div>
);

export const DetailOverviewCardReservesTranscript: Story = {
	beforeEach: resetOverviewCardStorage,
	render: () => (
		<OverviewCardReserveFrame>
			<AgentChatPageSkeleton />
		</OverviewCardReserveFrame>
	),
};

/** An embedded chat never shows the overview card, at any width. */
export const DetailEmbedded: Story = {
	beforeEach: resetOverviewCardStorage,
	render: () => (
		<EmbedContext value={{ isEmbedded: true }}>
			<OverviewCardReserveFrame>
				<AgentChatPageSkeleton />
			</OverviewCardReserveFrame>
		</EmbedContext>
	),
};
