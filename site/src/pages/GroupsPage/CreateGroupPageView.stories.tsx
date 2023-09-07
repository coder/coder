import { Story } from "@storybook/react";
import {
  CreateGroupPageView,
  CreateGroupPageViewProps,
} from "./CreateGroupPageView";

export default {
  title: "pages/CreateGroupPageView",
  component: CreateGroupPageView,
};

const Template: Story<CreateGroupPageViewProps> = (
  args: CreateGroupPageViewProps,
) => <CreateGroupPageView {...args} />;

export const Example = Template.bind({});
Example.args = {};
