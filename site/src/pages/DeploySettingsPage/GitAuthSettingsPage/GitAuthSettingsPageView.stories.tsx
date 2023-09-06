import { ComponentMeta, Story } from "@storybook/react";
import {
  GitAuthSettingsPageView,
  GitAuthSettingsPageViewProps,
} from "./GitAuthSettingsPageView";

export default {
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
} as ComponentMeta<typeof GitAuthSettingsPageView>;

const Template: Story<GitAuthSettingsPageViewProps> = (args) => (
  <GitAuthSettingsPageView {...args} />
);
export const Page = Template.bind({});
