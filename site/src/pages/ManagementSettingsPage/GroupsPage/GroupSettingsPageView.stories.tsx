import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import { MockGroup } from "testHelpers/entities";
import GroupSettingsPageView from "./GroupSettingsPageView";

const meta: Meta<typeof GroupSettingsPageView> = {
  title: "pages/GroupsPage/GroupSettingsPageView",
  component: GroupSettingsPageView,
  args: {
    onCancel: action("onCancel"),
    group: MockGroup,
    isLoading: false,
  },
};

export default meta;
type Story = StoryObj<typeof GroupSettingsPageView>;

const Example: Story = {};

export { Example as GroupSettingsPageView };
