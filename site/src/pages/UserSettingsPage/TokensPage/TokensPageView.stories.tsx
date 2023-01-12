import { Story } from "@storybook/react"
import { makeMockApiError } from "testHelpers/entities"
import { TokensPageView, TokensPageViewProps } from "./TokensPageView"

export default {
  title: "components/TokensPageView",
  component: TokensPageView,
  argTypes: {
    onRegenerateClick: { action: "Submit" },
  },
}

const Template: Story<TokensPageViewProps> = (args: TokensPageViewProps) => (
  <TokensPageView {...args} />
)

export const Example = Template.bind({})
Example.args = {
  isLoading: false,
  hasLoaded: true,
  tokens: [
    {
      id: "tBoVE3dqLl",
      user_id: "f9ee61d8-1d84-4410-ab6e-c1ec1a641e0b",
      last_used: "0001-01-01T00:00:00Z",
      expires_at: "2023-01-15T20:10:45.637438Z",
      created_at: "2022-12-16T20:10:45.637452Z",
      updated_at: "2022-12-16T20:10:45.637452Z",
      login_type: "token",
      scope: "all",
      lifetime_seconds: 2592000,
    },
    {
      id: "tBoVE3dqLl",
      user_id: "f9ee61d8-1d84-4410-ab6e-c1ec1a641e0b",
      last_used: "0001-01-01T00:00:00Z",
      expires_at: "2023-01-15T20:10:45.637438Z",
      created_at: "2022-12-16T20:10:45.637452Z",
      updated_at: "2022-12-16T20:10:45.637452Z",
      login_type: "token",
      scope: "all",
      lifetime_seconds: 2592000,
    },
  ],
  onDelete: () => {
    return Promise.resolve()
  },
}

export const Loading = Template.bind({})
Loading.args = {
  ...Example.args,
  isLoading: true,
  hasLoaded: false,
}

export const Empty = Template.bind({})
Empty.args = {
  ...Example.args,
  tokens: [],
}

export const WithGetTokensError = Template.bind({})
WithGetTokensError.args = {
  ...Example.args,
  hasLoaded: false,
  getTokensError: makeMockApiError({
    message: "Failed to get tokens.",
  }),
}

export const WithDeleteTokenError = Template.bind({})
WithDeleteTokenError.args = {
  ...Example.args,
  hasLoaded: false,
  deleteTokenError: makeMockApiError({
    message: "Failed to delete token.",
  }),
}
