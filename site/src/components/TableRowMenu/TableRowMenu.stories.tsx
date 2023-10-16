import { TableRowMenu } from "./TableRowMenu";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof TableRowMenu> = {
  title: "components/TableRowMenu",
  component: TableRowMenu,
};

export default meta;
type Story = StoryObj<typeof TableRowMenu<{ id: string }>>;

const Example: Story = {
  args: {
    data: { id: "123" },
    menuItems: [
      { label: "Suspend", onClick: (data) => alert(data.id), disabled: false },
      { label: "Update", onClick: (data) => alert(data.id), disabled: false },
      { label: "Delete", onClick: (data) => alert(data.id), disabled: false },
      { label: "Explode", onClick: (data) => alert(data.id), disabled: true },
    ],
  },
};

export { Example as TableRowMenu };
