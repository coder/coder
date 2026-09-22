import type { Meta, StoryObj } from "@storybook/react-vite";
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

export const EmptyConversationPrompt: Story = {};
