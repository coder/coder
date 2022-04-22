import { Story } from "@storybook/react"
import React from "react"
import { CreateUserForm, CreateUserFormProps } from "./CreateUserForm"

export default {
  title: "components/CreateUserForm",
  component: CreateUserForm,
  argTypes: {
    isLoading: "boolean",
    authErrorMessage: "string",
    onSubmit: { action: "Submit" },
  },
}

const Template: Story<CreateUserFormProps> = (args: CreateUserFormProps) => <CreateUserForm {...args} />

export const Example = Template.bind({})
Example.args = {
  onSubmit: () => {
    return Promise.resolve()
  },
}
