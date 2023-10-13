import { GroupAvatar } from "./GroupAvatar";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof GroupAvatar> = {
  title: "components/GroupAvatar",
  component: GroupAvatar,
};

export default meta;
type Story = StoryObj<typeof GroupAvatar>;

const Example: Story = {
  args: {
    name: "My Group",
    avatarURL: "",
  },
};

export { Example as GroupAvatar };
