import { Story } from "@storybook/react";
import { MockGroup } from "testHelpers/entities";
import { GroupsPageView, GroupsPageViewProps } from "./GroupsPageView";

export default {
  title: "pages/GroupsPageView",
  component: GroupsPageView,
};

const Template: Story<GroupsPageViewProps> = (args: GroupsPageViewProps) => (
  <GroupsPageView {...args} />
);

export const NotEnabled = Template.bind({});
NotEnabled.args = {
  groups: [MockGroup],
  canCreateGroup: true,
  isTemplateRBACEnabled: false,
};

export const WithGroups = Template.bind({});
WithGroups.args = {
  groups: [MockGroup],
  canCreateGroup: true,
  isTemplateRBACEnabled: true,
};

export const WithDisplayGroup = Template.bind({});
WithGroups.args = {
  groups: [{ ...MockGroup, name: "front-end" }],
  canCreateGroup: true,
  isTemplateRBACEnabled: true,
};

export const EmptyGroup = Template.bind({});
EmptyGroup.args = {
  groups: [],
  canCreateGroup: false,
  isTemplateRBACEnabled: true,
};

export const EmptyGroupWithPermission = Template.bind({});
EmptyGroupWithPermission.args = {
  groups: [],
  canCreateGroup: true,
  isTemplateRBACEnabled: true,
};
