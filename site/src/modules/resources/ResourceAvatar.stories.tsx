import { MockWorkspaceResource } from "testHelpers/entities";
import { ResourceAvatar } from "./ResourceAvatar";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof ResourceAvatar> = {
  title: "modules/resources/ResourceAvatar",
  component: ResourceAvatar,
};

export default meta;
type Story = StoryObj<typeof ResourceAvatar>;

export const VolumeResource: Story = {
  args: {
    resource: {
      ...MockWorkspaceResource,
      type: "docker_volume",
    },
  },
};

export const ComputeResource: Story = {
  args: {
    resource: {
      ...MockWorkspaceResource,
      type: "docker_container",
    },
  },
};

export const ImageResource: Story = {
  args: {
    resource: {
      ...MockWorkspaceResource,
      type: "docker_image",
    },
  },
};

export const NullResource: Story = {
  args: {
    resource: {
      ...MockWorkspaceResource,
      type: "null_resource",
    },
  },
};

export const UnknownResource: Story = {
  args: {
    resource: {
      ...MockWorkspaceResource,
      type: "noexistentvalue",
    },
  },
};

export const EmptyIcon: Story = {
  args: {
    resource: {
      ...MockWorkspaceResource,
      type: "helm_release",
      icon: "",
    },
  },
};
