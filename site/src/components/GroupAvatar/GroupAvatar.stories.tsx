import { Story } from "@storybook/react";
import { GroupAvatar, GroupAvatarProps } from "./GroupAvatar";

export default {
  title: "components/GroupAvatar",
  component: GroupAvatar,
};

const Template: Story<GroupAvatarProps> = (args) => <GroupAvatar {...args} />;

export const Example = Template.bind({});
Example.args = {
  name: "My Group",
  avatarURL: "",
};
