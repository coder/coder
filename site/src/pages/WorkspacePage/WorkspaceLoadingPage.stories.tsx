import type { Meta, StoryObj } from "@storybook/react";
import { MockWorkspace } from "testHelpers/entities";
import { WorkspaceLoadingPage } from "./WorkspaceLoadingPage";

const meta: Meta<typeof WorkspaceLoadingPage> = {
  title: "pages/WorkspacePage/WorkspaceLoadingPage",
  component: WorkspaceLoadingPage,
};

export default meta;
type Story = StoryObj<typeof WorkspaceLoadingPage>;

export const NoWorkspace: Story = {};

export const BuildPendingWithNoProvisioners: Story = {
  args: {
    workspace: {
      ...MockWorkspace,
      latest_build: {
        ...MockWorkspace.latest_build,
				status: "pending",
        matched_provisioners: {
          count: 0,
          available: 0,
        },
      },
    },
  },
};

export const BuildPendingWithUnavailableProvisioners: Story = {
  args: {
    workspace: {
      ...MockWorkspace,
      latest_build: {
        ...MockWorkspace.latest_build,
        status: "pending",
        matched_provisioners: {
          count: 1,
          available: 0,
        },
      },
    },
  },
};
