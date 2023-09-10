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
        },
      ],
    },
  },
};

export default meta;
type Story = StoryObj<typeof GitAuthSettingsPageView>;

export const Page: Story = {};
