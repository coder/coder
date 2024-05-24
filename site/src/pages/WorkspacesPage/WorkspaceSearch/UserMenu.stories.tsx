import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, within } from "@storybook/test";
import { useState } from "react";
import { UserMenu } from "./UserMenu";

const meta: Meta<typeof UserMenu> = {
  title: "pages/WorkspacesPage/UserMenu",
  component: UserMenu,
  parameters: {
    queries: [
      {
        key: ["users", {}],
        data: {
          users: [
            { id: "1", name: "Alice", username: "alice", avatar_url: "" },
            { id: "2", name: "Bob", username: "bob", avatar_url: "" },
            { id: "3", name: "Charlie", username: "charlie", avatar_url: "" },
          ],
        },
      },
    ],
  },
};

export default meta;
type Story = StoryObj<typeof UserMenu>;

export const Close: Story = {};

export const Open: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /Select user/i });
    await userEvent.click(button);
  },
};

export const Default: Story = {
  args: {
    selected: "2",
  },
};

export const SelectOption: Story = {
  render: function UserMenuWithState(args) {
    const [selected, setSelected] = useState<string | undefined>(undefined);
    return <UserMenu {...args} selected={selected} onSelect={setSelected} />;
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /Select user/i });
    await userEvent.click(button);
    const option = canvas.getByText("Charlie");
    await userEvent.click(option);
  },
};
