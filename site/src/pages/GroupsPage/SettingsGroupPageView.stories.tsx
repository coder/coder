import { Story } from "@storybook/react";
import { MockGroup } from "testHelpers/entities";
import {
  SettingsGroupPageView,
  SettingsGroupPageViewProps,
} from "./SettingsGroupPageView";

export default {
  title: "pages/SettingsGroupPageView",
  component: SettingsGroupPageView,
};

const Template: Story<SettingsGroupPageViewProps> = (
  args: SettingsGroupPageViewProps,
) => <SettingsGroupPageView {...args} />;

export const Example = Template.bind({});
Example.args = {
  group: MockGroup,
  isLoading: false,
};
