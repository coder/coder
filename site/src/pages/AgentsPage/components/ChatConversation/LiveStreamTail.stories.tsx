import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { LiveStreamTailContent } from "./LiveStreamTail";
import { buildLiveStatus, pinFixtureClock } from "./storyFixtures";

const meta: Meta<typeof LiveStreamTailContent> = {
	title: "pages/AgentsPage/ChatConversation/LiveStreamTail",
	component: LiveStreamTailContent,
	beforeEach: pinFixtureClock,
	args: {
		isTranscriptEmpty: true,
		liveStatus: buildLiveStatus(),
	},
};
export default meta;
type Story = StoryObj<typeof LiveStreamTailContent>;

export const EmptyConversationPrompt: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText(/start a conversation with your agent/i),
		).toBeVisible();
	},
};
