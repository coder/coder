import { Story } from "@storybook/react"
import { GitAuth, GitAuthProps } from "./GitAuth"

export default {
  title: "components/GitAuth",
  component: GitAuth,
}

const Template: Story<GitAuthProps> = (args) => <GitAuth {...args} />

export const GithubNotAuthenticated = Template.bind({})
GithubNotAuthenticated.args = {
  type: "github",
  authenticated: false,
}

export const GithubAuthenticated = Template.bind({})
GithubAuthenticated.args = {
  type: "github",
  authenticated: true,
}

export const GitlabNotAuthenticated = Template.bind({})
GitlabNotAuthenticated.args = {
  type: "gitlab",
  authenticated: false,
}

export const GitlabAuthenticated = Template.bind({})
GitlabAuthenticated.args = {
  type: "gitlab",
  authenticated: true,
}

export const AzureDevOpsNotAuthenticated = Template.bind({})
AzureDevOpsNotAuthenticated.args = {
  type: "azure-devops",
  authenticated: false,
}

export const AzureDevOpsAuthenticated = Template.bind({})
AzureDevOpsAuthenticated.args = {
  type: "azure-devops",
  authenticated: true,
}

export const BitbucketNotAuthenticated = Template.bind({})
BitbucketNotAuthenticated.args = {
  type: "bitbucket",
  authenticated: false,
}

export const BitbucketAuthenticated = Template.bind({})
BitbucketAuthenticated.args = {
  type: "bitbucket",
  authenticated: true,
}
