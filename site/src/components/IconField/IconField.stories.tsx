import { action } from "@storybook/addon-actions";
import IconField from "./IconField";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof IconField> = {
  title: "components/IconField",
  component: IconField,
  args: {
    onPickEmoji: action("onPickEmoji"),
  },
};

export default meta;
type Story = StoryObj<typeof IconField>;

export const Example: Story = {};

export const EmojiSelected: Story = {
  args: {
    value: "/emojis/1f3f3-fe0f-200d-26a7-fe0f.png",
  },
};

export const IconSelected: Story = {
  args: {
    value: "/icon/fedora.svg",
  },
};
