import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import { CreateUserForm, CreateUserFormProps } from "./CreateUserForm"

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

export const UnknownError = Template.bind({})
UnknownError.args = {
  onCancel: action("cancel"),
  onSubmit: action("submit"),
  isLoading: false,
  error: "Something went wrong",
}

export const FormError = Template.bind({})
FormError.args = {
  onCancel: action("cancel"),
  onSubmit: action("submit"),
  isLoading: false,
  formErrors: {
    username: "Username taken",
  },
}

export const Loading = Template.bind({})
Loading.args = {
  onCancel: action("cancel"),
  onSubmit: action("submit"),
  isLoading: true,
}
