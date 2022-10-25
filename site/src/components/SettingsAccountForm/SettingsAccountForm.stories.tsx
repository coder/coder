import { Story } from "@storybook/react"
import { AccountForm, AccountFormProps } from "./SettingsAccountForm"

export default {
  title: "components/SettingsAccountForm",
  component: AccountForm,
  argTypes: {
    onSubmit: { action: "Submit" },
  },
}

const Template: Story<AccountFormProps> = (args: AccountFormProps) => (
  <AccountForm {...args} />
)

export const Example = Template.bind({})
Example.args = {
  email: "test-user@org.com",
  isLoading: false,
  initialValues: {
    username: "test-user",
  },
  updateProfileError: undefined,
  onSubmit: () => {
    return Promise.resolve()
  },
}

export const Loading = Template.bind({})
Loading.args = {
  ...Example.args,
  isLoading: true,
}

export const WithError = Template.bind({})
WithError.args = {
  ...Example.args,
  updateProfileError: {
    response: {
      data: {
        message: "Username is invalid",
        validations: [
          {
            field: "username",
            detail: "Username is too long.",
          },
        ],
      },
    },
    isAxiosError: true,
  },
  initialTouched: {
    username: true,
  },
}
