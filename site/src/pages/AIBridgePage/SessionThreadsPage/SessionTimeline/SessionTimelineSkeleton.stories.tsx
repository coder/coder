import type { Meta, StoryObj } from "@storybook/react-vite";
import { SessionTimelineSkeleton } from "./SessionTimelineSkeleton";

const meta: Meta<typeof SessionTimelineSkeleton> = {
	title: "pages/AIBridgePage/SessionTimeline/SessionTimelineSkeleton",
	component: SessionTimelineSkeleton,
};

export default meta;
type Story = StoryObj<typeof SessionTimelineSkeleton>;

export const Default: Story = {};
