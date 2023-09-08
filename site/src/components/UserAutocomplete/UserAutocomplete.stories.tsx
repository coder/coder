import { MockUser } from "testHelpers/entities";
import { UserAutocomplete } from "./UserAutocomplete";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof UserAutocomplete> = {
  title: "components/UserAutocomplete",
  component: UserAutocomplete,
};

export default meta;
type Story = StoryObj<typeof UserAutocomplete>;

export const Example: Story = {
  args: {
    value: MockUser,
    label: "User",
  },
};

export const NoLabel: Story = {
  args: {
    value: MockUser,
  },
};
