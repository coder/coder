import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import { MockGroup } from "testHelpers/entities";
import { SettingsGroupPageView } from "./SettingsGroupPageView";

const meta: Meta<typeof SettingsGroupPageView> = {
  title: "pages/GroupsPage/SettingsGroupPageView",
  component: SettingsGroupPageView,
  args: {
    onCancel: action("onCancel"),
    group: MockGroup,
    isLoading: false,
  },
};

export default meta;
type Story = StoryObj<typeof SettingsGroupPageView>;

const Example: Story = {};

export { Example as SettingsGroupPageView };
