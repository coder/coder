import { ImageIcon } from "./ImageIcon";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof ImageIcon> = {
  title: "components/ImageIcon",
  component: ImageIcon,
  args: {
    children: <img src="/icon/docker.png" alt="" />,
    size: 24,
  },
};

export default meta;
type Story = StoryObj<typeof ImageIcon>;

export const Default: Story = {};
