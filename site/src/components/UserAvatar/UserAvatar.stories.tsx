import type { Meta, StoryObj } from "@storybook/react";
import { UserAvatar } from "./UserAvatar";

const meta: Meta<typeof UserAvatar> = {
  title: "components/UserAvatar",
  component: UserAvatar,
};

export default meta;
type Story = StoryObj<typeof UserAvatar>;

export const Jon: Story = {
  args: {
    username: "sreya",
    avatarURL: "https://github.com/sreya.png",
  },
};

export const Jonjon: Story = {
  args: {
    username: "ジョンジョン",
    avatarURL: "https://github.com/sreya.png",
  },
};
