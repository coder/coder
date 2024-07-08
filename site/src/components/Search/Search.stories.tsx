import type { Meta, StoryObj } from "@storybook/react";
import { Search, SearchInput } from "./Search";

const meta: Meta<typeof SearchInput> = {
  title: "components/Search",
  component: SearchInput,
  decorators: [
    (Story) => (
      <Search>
        <Story />
      </Search>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof SearchInput>;

export const Example: Story = {};

export const WithPlaceholder: Story = {
  args: {
    label: "uwu",
    placeholder: "uwu",
  },
};
