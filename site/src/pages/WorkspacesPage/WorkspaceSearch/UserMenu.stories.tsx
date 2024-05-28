import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, within } from "@storybook/test";
import { useState } from "react";
import type { User } from "api/typesGenerated";
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

export const ManyOptions: Story = {
  parameters: {
    queries: [
      {
        key: ["users", {}],
        data: {
          users: generateUsers(50),
        },
      },
    ],
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /Select user/i });
    await userEvent.click(button);
  },
};

export const SearchStickyOnTop: Story = {
  parameters: {
    queries: [
      {
        key: ["users", {}],
        data: {
          users: generateUsers(50),
        },
      },
    ],
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /Select user/i });
    await userEvent.click(button);

    const content = canvasElement.querySelector(".MuiPaper-root");
    content?.scrollTo(0, content.scrollHeight);
  },
};

export const ScrollToSelectedOption: Story = {
  args: {
    selected: "50",
  },
  parameters: {
    queries: [
      {
        key: ["users", {}],
        data: {
          users: generateUsers(50),
        },
      },
    ],
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /Select user/i });
    await userEvent.click(button);
  },
};

export const Filter: Story = {
  parameters: {
    queries: [
      {
        key: ["users", {}],
        data: {
          users: generateUsers(50),
        },
      },
    ],
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /Select user/i });
    await userEvent.click(button);
    const filter = canvas.getByLabelText("Search user");
    await userEvent.type(filter, "user23@coder.com");
  },
};

export const EmptyResults: Story = {
  parameters: {
    queries: [
      {
        key: ["users", {}],
        data: {
          users: generateUsers(50),
        },
      },
    ],
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /Select user/i });
    await userEvent.click(button);
    const filter = canvas.getByLabelText("Search user");
    await userEvent.type(filter, "user1020@coder.com");
  },
};

export const FocusOnFirstResultWhenPressArrowDown: Story = {
  parameters: {
    queries: [
      {
        key: ["users", {}],
        data: {
          users: generateUsers(50),
        },
      },
    ],
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /Select user/i });
    await userEvent.click(button);
    const filter = canvas.getByLabelText("Search user");
    await userEvent.type(filter, "user1");
    await userEvent.type(filter, "{arrowdown}");
  },
};

function generateUsers(amount: number): Partial<User>[] {
  return Array.from({ length: amount }, (_, i) => ({
    id: i.toString(),
    name: `User ${i}`,
    username: `user${i}`,
    avatar_url: "",
    email: `user${i}@coder.com`,
  }));
}
