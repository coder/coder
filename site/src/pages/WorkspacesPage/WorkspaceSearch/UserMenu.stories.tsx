import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, within } from "@storybook/test";
import { useState } from "react";
import type { User } from "api/typesGenerated";
import { UserMenu } from "./UserMenu";

const defaultQueries = [
  {
    key: ["users", { limit: 100, q: "" }],
    data: {
      users: generateUsers(50),
    },
  },
];

const meta: Meta<typeof UserMenu> = {
  title: "pages/WorkspacesPage/UserMenu",
  component: UserMenu,
  parameters: {
    queries: defaultQueries,
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

export const Selected: Story = {
  args: {
    selected: user(2).email,
  },
  parameters: {
    queries: [
      {
        key: ["users", { limit: 1, q: user(2).email }],
        data: user(2),
      },
    ],
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
    const option = canvas.getByText("User 4");
    await userEvent.click(option);
  },
  parameters: {
    queries: [
      ...defaultQueries,
      {
        key: ["users", { limit: 1, q: user(4).email }],
        data: user(4),
      },
    ],
  },
};

export const SearchStickyOnTop: Story = {
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
    selected: user(30).email,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /Select user/i });
    await userEvent.click(button);
  },
  parameters: {
    queries: [
      ...defaultQueries,
      {
        key: ["users", { limit: 1, q: user(30).email }],
        data: user(30),
      },
    ],
  },
};

export const Filter: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /Select user/i });
    await userEvent.click(button);
    const filter = canvas.getByLabelText("Search user");
    await userEvent.type(filter, user(23).email!);
  },
  parameters: {
    queries: [
      ...defaultQueries,
      {
        key: ["users", { limit: 100, q: user(23).email }],
        data: {
          users: [user(23)],
        },
      },
    ],
  },
};

export const EmptyResults: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /Select user/i });
    await userEvent.click(button);
    const filter = canvas.getByLabelText("Search user");
    await userEvent.type(filter, "invalid-user@coder.com");
  },
  parameters: {
    queries: [
      ...defaultQueries,
      {
        key: ["users", { limit: 100, q: "invalid-user@coder.com" }],
        data: {
          users: [],
        },
      },
    ],
  },
};

export const FocusOnFirstResultWhenPressArrowDown: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /Select user/i });
    await userEvent.click(button);
    const filter = canvas.getByLabelText("Search user");
    await userEvent.type(filter, user(1).email!);
    await userEvent.type(filter, "{arrowdown}");
  },
  parameters: {
    queries: [
      ...defaultQueries,
      {
        key: ["users", { limit: 100, q: user(1).email }],
        data: {
          users: [user(1)],
        },
      },
    ],
  },
};

function generateUsers(amount: number): Partial<User>[] {
  return Array.from({ length: amount }, (_, i) => user(i));
}

function user(i: number): Partial<User> {
  return {
    id: i.toString(),
    name: `User ${i}`,
    username: `user${i}`,
    avatar_url: "",
    email: `user${i}@coder.com`,
  };
}
