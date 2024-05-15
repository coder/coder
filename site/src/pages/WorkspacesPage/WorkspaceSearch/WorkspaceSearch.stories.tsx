import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, within, expect } from "@storybook/test";
import { useState } from "react";
import { WorkspaceSearch } from "./WorkspaceSearch";

const meta: Meta<typeof WorkspaceSearch> = {
  title: "pages/WorkspacesPage/WorkspaceSearch",
  component: WorkspaceSearch,
  args: {
    query: "",
  },
  render: function WorkspaceSearchWithState(args) {
    const [query, setQuery] = useState<string>(args.query);
    return <WorkspaceSearch {...args} query={query} setQuery={setQuery} />;
  },
};

export default meta;
type Story = StoryObj<typeof WorkspaceSearch>;

export const SelectPresetFilter: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /filters/i });
    await userEvent.click(button);
    const option = canvas.getByText("Failed workspaces");
    await userEvent.click(option);
    const input = canvas.getByLabelText("Search workspace");
    await expect(input).toHaveValue("failed:true");
  },
};
