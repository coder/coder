import { Story } from "@storybook/react"
import React from "react"
import * as Mocks from "../../testHelpers/renderHelpers"
import { TemplateStats, TemplateStatsProps } from "../TemplateStats/TemplateStats"

export default {
  title: "components/TemplateStats",
  component: TemplateStats,
}

const Template: Story<TemplateStatsProps> = (args) => <TemplateStats {...args} />

export const Example = Template.bind({})
Example.args = {
  template: Mocks.MockTemplate,
  activeVersion: Mocks.MockTemplateVersion,
}

export const UsedByMany = Template.bind({})
UsedByMany.args = {
  template: {
    ...Mocks.MockTemplate,
    workspace_owner_count: 15,
  },
  activeVersion: Mocks.MockTemplateVersion,
}
