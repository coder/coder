import { Story } from "@storybook/react";
import { MockUser } from "testHelpers/entities";
import { UserAutocomplete, UserAutocompleteProps } from "./UserAutocomplete";

export default {
  title: "components/UserAutocomplete",
  component: UserAutocomplete,
};

const Template: Story<UserAutocompleteProps> = (
  args: UserAutocompleteProps,
) => <UserAutocomplete {...args} />;

export const Example = Template.bind({});
Example.args = {
  value: MockUser,
  label: "User",
};

export const NoLabel = Template.bind({});
NoLabel.args = {
  value: MockUser,
};
