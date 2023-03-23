import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import * as Mocks from "../../../testHelpers/renderHelpers"
import { makeMockApiError } from "../../../testHelpers/renderHelpers"
import {
  TemplateSettingsPageView,
  TemplateSettingsPageViewProps,
} from "./TemplateSettingsPageView"

export default {
  title: "pages/TemplateSettingsPageView",
  component: TemplateSettingsPageView,
  args: {
    template: Mocks.MockTemplate,
    onSubmit: action("onSubmit"),
    onCancel: action("cancel"),
  },
}

const Template: Story<TemplateSettingsPageViewProps> = (args) => (
  <TemplateSettingsPageView {...args} />
)

export const Example = Template.bind({})
Example.args = {}

export const SaveTemplateSettingsError = Template.bind({})
SaveTemplateSettingsError.args = {
  submitError: makeMockApiError({
    message: 'Template "test" already exists.',
    validations: [
      {
        field: "name",
        detail: "This value is already in use and should be unique.",
      },
    ],
  }),
  initialTouched: {
    allow_user_cancel_workspace_jobs: true,
  },
}
