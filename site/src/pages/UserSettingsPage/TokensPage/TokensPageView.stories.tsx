import { Story } from "@storybook/react"
import { makeMockApiError, MockTokens } from "testHelpers/entities"
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
  tokens: MockTokens,
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
