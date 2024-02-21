import { TemplateVersionExternalAuth } from "api/typesGenerated";
import { ExternalAuthButton } from "./ExternalAuthButton";
import type { Meta, StoryObj } from "@storybook/react";

const MockExternalAuth: TemplateVersionExternalAuth = {
  id: "",
  type: "",
  display_name: "GitHub",
  display_icon: "/icon/github.svg",
  authenticate_url: "",
  authenticated: false,
};

const meta: Meta<typeof ExternalAuthButton> = {
  title: "pages/CreateWorkspacePage/ExternalAuth",
  component: ExternalAuthButton,
};

export default meta;
type Story = StoryObj<typeof ExternalAuthButton>;

export const Github: Story = {
  args: {
    auth: MockExternalAuth,
  },
};

export const GithubWithRetry: Story = {
  args: {
    auth: MockExternalAuth,
    displayRetry: true,
  },
};

export const GithubAuthenticated: Story = {
  args: {
    auth: {
      ...MockExternalAuth,
      authenticated: true,
    },
  },
};

export const Gitlab: Story = {
  args: {
    auth: {
      ...MockExternalAuth,
      display_icon: "/icon/gitlab.svg",
      display_name: "GitLab",
      authenticated: false,
    },
  },
};

export const GitlabAuthenticated: Story = {
  args: {
    auth: {
      ...MockExternalAuth,
      display_icon: "/icon/gitlab.svg",
      display_name: "GitLab",
      authenticated: true,
    },
  },
};

export const AzureDevOps: Story = {
  args: {
    auth: {
      ...MockExternalAuth,
      display_icon: "/icon/azure-devops.svg",
      display_name: "Azure DevOps",
      authenticated: false,
    },
  },
};

export const AzureDevOpsAuthenticated: Story = {
  args: {
    auth: {
      ...MockExternalAuth,
      display_icon: "/icon/azure-devops.svg",
      display_name: "Azure DevOps",
      authenticated: true,
    },
  },
};

export const Bitbucket: Story = {
  args: {
    auth: {
      ...MockExternalAuth,
      display_icon: "/icon/bitbucket.svg",
      display_name: "Bitbucket",
      authenticated: false,
    },
  },
};

export const BitbucketAuthenticated: Story = {
  args: {
    auth: {
      ...MockExternalAuth,
      display_icon: "/icon/bitbucket.svg",
      display_name: "Bitbucket",
      authenticated: true,
    },
  },
};
