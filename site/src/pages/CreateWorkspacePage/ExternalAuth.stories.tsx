import { ExternalAuth } from "./ExternalAuth";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof ExternalAuth> = {
  title: "components/ExternalAuth",
  component: ExternalAuth,
};

export default meta;
type Story = StoryObj<typeof ExternalAuth>;

export const GithubNotAuthenticated: Story = {
  args: {
    displayIcon: "/icon/github.svg",
    displayName: "GitHub",
    authenticated: false,
  },
};

export const GithubAuthenticated: Story = {
  args: {
    displayIcon: "/icon/github.svg",
    displayName: "GitHub",
    authenticated: true,
  },
};

export const GitlabNotAuthenticated: Story = {
  args: {
    displayIcon: "/icon/gitlab.svg",
    displayName: "GitLab",
    authenticated: false,
  },
};

export const GitlabAuthenticated: Story = {
  args: {
    displayIcon: "/icon/gitlab.svg",
    displayName: "GitLab",
    authenticated: true,
  },
};

export const AzureDevOpsNotAuthenticated: Story = {
  args: {
    displayIcon: "/icon/azure-devops.svg",
    displayName: "Azure DevOps",
    authenticated: false,
  },
};

export const AzureDevOpsAuthenticated: Story = {
  args: {
    displayIcon: "/icon/azure-devops.svg",
    displayName: "Azure DevOps",
    authenticated: true,
  },
};

export const BitbucketNotAuthenticated: Story = {
  args: {
    displayIcon: "/icon/bitbucket.svg",
    displayName: "Bitbucket",
    authenticated: false,
  },
};

export const BitbucketAuthenticated: Story = {
  args: {
    displayIcon: "/icon/bitbucket.svg",
    displayName: "Bitbucket",
    authenticated: true,
  },
};
