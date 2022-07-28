import { Story } from "@storybook/react"
import { SSHKeysPageView, SSHKeysPageViewProps } from "./SSHKeysPageView"

export default {
  title: "components/SSHKeysPageView",
  component: SSHKeysPageView,
  argTypes: {
    onRegenerateClick: { action: "Submit" },
  },
}

const Template: Story<SSHKeysPageViewProps> = (args: SSHKeysPageViewProps) => (
  <SSHKeysPageView {...args} />
)

export const Example = Template.bind({})
Example.args = {
  isLoading: false,
  hasLoaded: true,
  sshKey: {
    user_id: "test-user-id",
    created_at: "2022-07-28T07:45:50.795918897Z",
    updated_at: "2022-07-28T07:45:50.795919142Z",
    public_key: "SSH-Key",
  },
  onRegenerateClick: () => {
    return Promise.resolve()
  },
}

export const Loading = Template.bind({})
Loading.args = {
  ...Example.args,
  isLoading: true,
}

export const WithGetSSHKeyError = Template.bind({})
WithGetSSHKeyError.args = {
  ...Example.args,
  hasLoaded: false,
  getSSHKeyError: {
    response: {
      data: {
        message: "Failed to get SSH key",
      },
    },
    isAxiosError: true,
  },
}

export const WithRegenerateSSHKeyError = Template.bind({})
WithRegenerateSSHKeyError.args = {
  ...Example.args,
  regenerateSSHKeyError: {
    response: {
      data: {
        message: "Failed to regenerate SSH key",
      },
    },
    isAxiosError: true,
  },
}
