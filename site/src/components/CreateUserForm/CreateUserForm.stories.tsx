import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import { CreateUserForm, CreateUserFormProps } from "./CreateUserForm"
import { mockApiError } from "testHelpers/entities"

export default {
  title: "components/CreateUserForm",
  component: CreateUserForm,
}

const Template: Story<CreateUserFormProps> = (args: CreateUserFormProps) => (
  <CreateUserForm {...args} />
)

export const Ready = Template.bind({})
Ready.args = {
  onCancel: action("cancel"),
  onSubmit: action("submit"),
  isLoading: false,
}

export const FormError = Template.bind({})
FormError.args = {
  onCancel: action("cancel"),
  onSubmit: action("submit"),
  isLoading: false,
  error: mockApiError({
    validations: [{ field: "username", detail: "Username taken" }],
  }),
}

export const Loading = Template.bind({})
Loading.args = {
  onCancel: action("cancel"),
  onSubmit: action("submit"),
  isLoading: true,
}
