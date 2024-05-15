import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, within } from "@storybook/test";
import { useState } from "react";
import { StatusMenu } from "./StatusMenu";

const meta: Meta<typeof StatusMenu> = {
  title: "pages/WorkspacesPage/StatusMenu",
  component: StatusMenu,
  args: {
    placeholder: "All statuses",
  },
};

export default meta;
type Story = StoryObj<typeof StatusMenu>;

export const Close: Story = {};

export const Open: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /Select status/i });
    await userEvent.click(button);
  },
};

export const Selected: Story = {
  args: {
    selected: "running",
  },
};

export const SelectedOpen: Story = {
  args: {
    selected: "running",
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /Select status/i });
    await userEvent.click(button);
  },
};

export const SelectOption: Story = {
  render: function StatusMenuWithState(args) {
    const [selected, setSelected] = useState<string | undefined>(undefined);
    return <StatusMenu {...args} selected={selected} onSelect={setSelected} />;
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /Select status/i });
    await userEvent.click(button);
    const option = canvas.getByText("Failed");
    await userEvent.click(option);
  },
};
