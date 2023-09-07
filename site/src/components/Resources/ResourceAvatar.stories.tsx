import { Story } from "@storybook/react";
import { MockWorkspaceResource } from "testHelpers/entities";
import { ResourceAvatar, ResourceAvatarProps } from "./ResourceAvatar";

export default {
  title: "components/ResourceAvatar",
  component: ResourceAvatar,
};

const Template: Story<ResourceAvatarProps> = (args) => (
  <ResourceAvatar {...args} />
);

export const VolumeResource = Template.bind({});
VolumeResource.args = {
  resource: {
    ...MockWorkspaceResource,
    type: "docker_volume",
  },
};

export const ComputeResource = Template.bind({});
ComputeResource.args = {
  resource: {
    ...MockWorkspaceResource,
    type: "docker_container",
  },
};

export const ImageResource = Template.bind({});
ImageResource.args = {
  resource: {
    ...MockWorkspaceResource,
    type: "docker_image",
  },
};

export const NullResource = Template.bind({});
NullResource.args = {
  resource: {
    ...MockWorkspaceResource,
    type: "null_resource",
  },
};

export const UnknownResource = Template.bind({});
UnknownResource.args = {
  resource: {
    ...MockWorkspaceResource,
    type: "noexistentvalue",
  },
};

export const EmptyIcon = Template.bind({});
EmptyIcon.args = {
  resource: {
    ...MockWorkspaceResource,
    type: "helm_release",
    icon: "",
  },
};
