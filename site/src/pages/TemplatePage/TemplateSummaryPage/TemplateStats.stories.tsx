import { Story } from "@storybook/react"
import { MockTemplate, MockTemplateVersion } from "testHelpers/entities"
import { TemplateStats, TemplateStatsProps } from "./TemplateStats"

export default {
  title: "components/TemplateStats",
  component: TemplateStats,
}

const Template: Story<TemplateStatsProps> = (args) => (
  <TemplateStats {...args} />
)

export const Example = Template.bind({})
Example.args = {
  template: MockTemplate,
  activeVersion: MockTemplateVersion,
}

export const UsedByMany = Template.bind({})
UsedByMany.args = {
  template: {
    ...MockTemplate,
    active_user_count: 15,
  },
  activeVersion: MockTemplateVersion,
}

export const ActiveUsersNotLoaded = Template.bind({})
ActiveUsersNotLoaded.args = {
  template: {
    ...MockTemplate,
    active_user_count: -1,
  },
  activeVersion: MockTemplateVersion,
}

export const LongTemplateVersion = Template.bind({})
LongTemplateVersion.args = {
  template: MockTemplate,
  activeVersion: {
    ...MockTemplateVersion,
    name: "thisisareallyreallylongnamefortesting",
  },
}
LongTemplateVersion.parameters = {
  chromatic: { viewports: [960] },
}

export const SmallViewport = Template.bind({})
SmallViewport.args = {
  template: MockTemplate,
  activeVersion: MockTemplateVersion,
}
SmallViewport.parameters = {
  chromatic: { viewports: [600] },
}
