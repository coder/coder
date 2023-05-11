import { Story } from "@storybook/react"
import {
  TemplateVersionWarnings,
  TemplateVersionWarningsProps,
} from "./TemplateVersionWarnings"

export default {
  title: "components/TemplateVersionWarnings",
  component: TemplateVersionWarnings,
}

const Template: Story<TemplateVersionWarningsProps> = (args) => (
  <TemplateVersionWarnings {...args} />
)

export const DeprecatedParameters = Template.bind({})
DeprecatedParameters.args = {
  warnings: ["DEPRECATED_PARAMETERS"],
}
