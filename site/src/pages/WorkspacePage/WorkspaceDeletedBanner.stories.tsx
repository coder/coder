import { Meta, StoryObj } from "@storybook/react";
import { WorkspaceDeletedBanner } from "./WorkspaceDeletedBanner";

const meta: Meta<typeof WorkspaceDeletedBanner> = {
  title: "components/WorkspaceDeletedBanner",
  component: WorkspaceDeletedBanner,
};

export default meta;
type Story = StoryObj<typeof WorkspaceDeletedBanner>;

export const Example: Story = {};
