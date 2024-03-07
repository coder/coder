import type { Meta, StoryObj } from "@storybook/react";
import { MockWorkspaceBuild } from "testHelpers/entities";
import { BuildAvatar } from "./BuildAvatar";

const meta: Meta<typeof BuildAvatar> = {
  title: "components/BuildAvatar",
  component: BuildAvatar,
  args: {
    build: MockWorkspaceBuild,
  },
};

export default meta;
type Story = StoryObj<typeof BuildAvatar>;

export const XSSize: Story = {
  args: {
    size: "xs",
  },
};

export const SMSize: Story = {
  args: {
    size: "sm",
  },
};

export const MDSize: Story = {
  args: {
    size: "md",
  },
};

export const XLSize: Story = {
  args: {
    size: "xl",
  },
};
