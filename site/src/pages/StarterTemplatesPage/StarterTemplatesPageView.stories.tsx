import { Story } from "@storybook/react"
import {
  makeMockApiError,
  MockOrganization,
  MockTemplateExample,
  MockTemplateExample2,
} from "testHelpers/entities"
import { getTemplatesByTag } from "util/starterTemplates"
import {
  StarterTemplatesPageView,
  StarterTemplatesPageViewProps,
} from "./StarterTemplatesPageView"

export default {
  title: "pages/StarterTemplatesPageView",
  component: StarterTemplatesPageView,
}

const Template: Story<StarterTemplatesPageViewProps> = (args) => (
  <StarterTemplatesPageView {...args} />
)

export const Default = Template.bind({})
Default.args = {
  context: {
    organizationId: MockOrganization.id,
    error: undefined,
    starterTemplatesByTag: getTemplatesByTag([
      MockTemplateExample,
      MockTemplateExample2,
    ]),
  },
}

export const Error = Template.bind({})
Error.args = {
  context: {
    organizationId: MockOrganization.id,
    error: makeMockApiError({
      message: "Error on loading the template examples",
    }),
    starterTemplatesByTag: undefined,
  },
}
