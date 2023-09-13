import { GitAuthSettingsPageView } from "./GitAuthSettingsPageView";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof GitAuthSettingsPageView> = {
  title: "pages/GitAuthSettingsPageView",
  component: GitAuthSettingsPageView,
  args: {
    config: {
      git_auth: [
        {
          id: "0000-1111",
          type: "GitHub",
          client_id: "client_id",
          regex: "regex",
          auth_url: "",
          token_url: "",
          validate_url: "",
          app_install_url: "https://github.com/apps/coder/installations/new",
          app_installations_url: "",
          no_refresh: false,
          scopes: [],
          device_flow: true,
          device_code_url: "",
        },
      ],
    },
  },
};

export default meta;
type Story = StoryObj<typeof GitAuthSettingsPageView>;

export const Page: Story = {};
