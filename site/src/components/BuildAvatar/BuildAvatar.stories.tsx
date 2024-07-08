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

export const Start: Story = {
  args: {
    build: {
      ...MockWorkspaceBuild,
      transition: "start",
    },
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

export const Succeeded: Story = {
  args: {
    build: {
      ...MockWorkspaceBuild,
      job: {
        ...MockWorkspaceBuild.job,
        status: "succeeded",
      },
    },
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
