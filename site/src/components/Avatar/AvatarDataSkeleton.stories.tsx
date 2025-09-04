import type { Meta, StoryObj } from "@storybook/react-vite";
import { AvatarDataSkeleton } from "./AvatarDataSkeleton";

const meta: Meta<typeof AvatarDataSkeleton> = {
	title: "components/AvatarDataSkeleton",
	component: AvatarDataSkeleton,
};

export default meta;
type Story = StoryObj<typeof AvatarDataSkeleton>;

export const Default: Story = {};
