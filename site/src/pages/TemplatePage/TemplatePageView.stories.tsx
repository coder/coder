import { ComponentMeta, Story } from "@storybook/react"
import React from "react"
import { TemplatePageView, TemplatePageViewProps } from "./TemplatePageView"

export default {
  title: "pages/TemplatesPageView",
  component: TemplatePageView,
} as ComponentMeta<typeof TemplatePageView>

const Template: Story<TemplatePageViewProps> = (args) => <TemplatePageView {...args} />

export const Empty = Template.bind({})
Empty.args = {}
