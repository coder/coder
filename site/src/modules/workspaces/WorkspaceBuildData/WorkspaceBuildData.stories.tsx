import type { Meta, StoryObj } from "@storybook/react";
import { MockWorkspaceBuild } from "testHelpers/entities";
import { WorkspaceBuildData } from "./WorkspaceBuildData";

const meta: Meta<typeof WorkspaceBuildData> = {
  title: "modules/workspaces/WorkspaceBuildData",
  component: WorkspaceBuildData,
};

export default meta;
type Story = StoryObj<typeof WorkspaceBuildData>;

export const Start: Story = {
  args: {
    build: MockWorkspaceBuild,
  },
};

export const Stop: Story = {
  args: {
    build: {
      ...MockWorkspaceBuild,
      transition: "stop",
    },
  },
};

export const Delete: Story = {
  args: {
    build: {
      ...MockWorkspaceBuild,
      transition: "delete",
    },
  },
};

export const Success: Story = {
  args: {
    build: MockWorkspaceBuild,
  },
};

export const Pending: Story = {
  args: {
    build: {
      ...MockWorkspaceBuild,
      job: {
        ...MockWorkspaceBuild.job,
        status: "pending",
      },
    },
  },
};

export const Running: Story = {
  args: {
    build: {
      ...MockWorkspaceBuild,
      job: {
        ...MockWorkspaceBuild.job,
        status: "running",
      },
    },
  },
};

export const Failed: Story = {
  args: {
    build: {
      ...MockWorkspaceBuild,
      job: {
        ...MockWorkspaceBuild.job,
        status: "failed",
      },
    },
  },
};

export const Canceling: Story = {
  args: {
    build: {
      ...MockWorkspaceBuild,
      job: {
        ...MockWorkspaceBuild.job,
        status: "canceling",
      },
    },
  },
};

export const Canceled: Story = {
  args: {
    build: {
      ...MockWorkspaceBuild,
      job: {
        ...MockWorkspaceBuild.job,
        status: "canceled",
      },
    },
  },
};
