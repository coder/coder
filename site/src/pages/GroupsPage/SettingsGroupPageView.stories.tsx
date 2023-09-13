import { MockGroup } from "testHelpers/entities";
import { SettingsGroupPageView } from "./SettingsGroupPageView";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof SettingsGroupPageView> = {
  title: "pages/SettingsGroupPageView",
  component: SettingsGroupPageView,
};

export default meta;
type Story = StoryObj<typeof SettingsGroupPageView>;

export const Example: Story = {
  args: {
    group: MockGroup,
    isLoading: false,
  },
};
