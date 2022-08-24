import { Story } from "@storybook/react"
import { ResourceAvatar, ResourceAvatarProps } from "./ResourceAvatar"

export default {
  title: "components/ResourceAvatar",
  component: ResourceAvatar,
}

const Template: Story<ResourceAvatarProps> = (args) => <ResourceAvatar {...args} />

export const VolumeResource = Template.bind({})
VolumeResource.args = {
  type: "docker_volume",
}

export const ComputeResource = Template.bind({})
ComputeResource.args = {
  type: "docker_container",
}

export const ImageResource = Template.bind({})
ImageResource.args = {
  type: "docker_image",
}

export const NullResource = Template.bind({})
NullResource.args = {
  type: "null_resource",
}

export const UnknownResource = Template.bind({})
UnknownResource.args = {
  type: "noexistentvalue",
}
