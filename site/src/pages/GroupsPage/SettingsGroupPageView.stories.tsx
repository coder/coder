import type { Meta, StoryObj } from "@storybook/react";
import { MockGroup } from "testHelpers/entities";
import { SettingsGroupPageView } from "./SettingsGroupPageView";

const meta: Meta<typeof SettingsGroupPageView> = {
  title: "pages/GroupsPage/SettingsGroupPageView",
  component: SettingsGroupPageView,
};

export default meta;
type Story = StoryObj<typeof SettingsGroupPageView>;

const Example: Story = {
  args: {
    group: MockGroup,
    isLoading: false,
  },
};

export { Example as SettingsGroupPageView };
