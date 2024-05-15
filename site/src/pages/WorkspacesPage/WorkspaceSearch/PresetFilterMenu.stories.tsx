import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, within } from "@storybook/test";
import { PresetFiltersMenu } from "./PresetFiltersMenu";

const meta: Meta<typeof PresetFiltersMenu> = {
  title: "pages/WorkspacesPage/PresetFiltersMenu",
  component: PresetFiltersMenu,
};

export default meta;
type Story = StoryObj<typeof PresetFiltersMenu>;

export const Closed: Story = {};

export const Opened: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /filters/i });
    await userEvent.click(button);
  },
};

export const CloseOnClick: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /filters/i });
    await userEvent.click(button);
    const option = canvas.getByText("All workspaces");
    await userEvent.click(option);
  },
};
