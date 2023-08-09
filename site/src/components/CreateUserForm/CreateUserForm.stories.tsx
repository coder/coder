import { action } from "@storybook/addon-actions"
import { StoryObj, Meta } from "@storybook/react"
import { CreateUserForm } from "./CreateUserForm"
import { mockApiError } from "testHelpers/entities"

const meta: Meta<typeof CreateUserForm> = {
  title: "components/CreateUserForm",
  component: CreateUserForm,
  args: {
    onCancel: action("cancel"),
    onSubmit: action("submit"),
    isLoading: false,
  },
}

export default meta
type Story = StoryObj<typeof CreateUserForm>

export const Ready: Story = {}

export const FormError: Story = {
  args: {
    error: mockApiError({
      validations: [{ field: "username", detail: "Username taken" }],
    }),
  },
}

export const GeneralError: Story = {
  args: {
    error: mockApiError({
      message: "User already exists",
    }),
  },
}

export const Loading: Story = {
  args: {
    isLoading: true,
  },
}
