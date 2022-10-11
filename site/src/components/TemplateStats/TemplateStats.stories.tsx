import { Story } from "@storybook/react"
import * as Mocks from "../../testHelpers/renderHelpers"
import {
  TemplateStats,
  TemplateStatsProps,
} from "../TemplateStats/TemplateStats"

export default {
  title: "components/TemplateStats",
  component: TemplateStats,
}

const Template: Story<TemplateStatsProps> = (args) => (
  <TemplateStats {...args} />
)

export const Example = Template.bind({})
Example.args = {
  template: Mocks.MockTemplate,
  activeVersion: Mocks.MockTemplateVersion,
}

export const UsedByMany = Template.bind({})
UsedByMany.args = {
  template: {
    ...Mocks.MockTemplate,
    active_user_count: 15,
  },
  activeVersion: Mocks.MockTemplateVersion,
}

export const ActiveUsersNotLoaded = Template.bind({})
ActiveUsersNotLoaded.args = {
  template: {
    ...Mocks.MockTemplate,
    active_user_count: -1,
  },
  activeVersion: Mocks.MockTemplateVersion,
}

export const LongTemplateVersion = Template.bind({})
LongTemplateVersion.args = {
  template: Mocks.MockTemplate,
  activeVersion: {
    ...Mocks.MockTemplateVersion,
    name: "thisisareallyreallylongnamefortesting",
  },
}
LongTemplateVersion.parameters = {
  chromatic: { viewports: [960] },
}

export const SmallViewport = Template.bind({})
SmallViewport.args = {
  template: Mocks.MockTemplate,
  activeVersion: Mocks.MockTemplateVersion,
}
SmallViewport.parameters = {
  chromatic: { viewports: [600] },
}
