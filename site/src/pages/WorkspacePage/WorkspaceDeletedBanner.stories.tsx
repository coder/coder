import type { Meta, StoryObj } from "@storybook/react";
import { WorkspaceDeletedBanner } from "./WorkspaceDeletedBanner";

const meta: Meta<typeof WorkspaceDeletedBanner> = {
  title: "pages/WorkspacePage/WorkspaceDeletedBanner",
  component: WorkspaceDeletedBanner,
};

export default meta;
type Story = StoryObj<typeof WorkspaceDeletedBanner>;

const Example: Story = {};

export { Example as WorkspaceDeletedBanner };
