import { ExternalAuthSettingsPageView } from "./ExternalAuthSettingsPageView";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof ExternalAuthSettingsPageView> = {
  title: "pages/ExternalAuthSettingsPageView",
  component: ExternalAuthSettingsPageView,
  args: {
    config: {
      external_auth: [
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
          display_icon: "",
          display_name: "GitHub",
        },
      ],
    },
  },
};

export default meta;
type Story = StoryObj<typeof ExternalAuthSettingsPageView>;

export const Page: Story = {};
