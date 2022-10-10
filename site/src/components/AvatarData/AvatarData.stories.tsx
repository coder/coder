import { Story } from "@storybook/react"
import { AvatarData, AvatarDataProps } from "./AvatarData"

export default {
  title: "components/AvatarData",
  component: AvatarData,
}

const Template: Story<AvatarDataProps> = (args: AvatarDataProps) => (
  <AvatarData {...args} />
)

export const Example = Template.bind({})
Example.args = {
  title: "coder",
  subtitle: "coder@coder.com",
}

export const WithHighlightTitle = Template.bind({})
WithHighlightTitle.args = {
  title: "coder",
  subtitle: "coder@coder.com",
  highlightTitle: true,
}

export const WithLink = Template.bind({})
WithLink.args = {
  title: "coder",
  subtitle: "coder@coder.com",
  link: "/users/coder",
}
