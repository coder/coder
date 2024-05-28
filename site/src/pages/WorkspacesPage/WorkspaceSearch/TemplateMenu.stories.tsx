import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, within } from "@storybook/test";
import { useState } from "react";
import { getTemplatesQueryKey } from "api/queries/templates";
import type { Template } from "api/typesGenerated";
import { TemplateMenu } from "./TemplateMenu";

const meta: Meta<typeof TemplateMenu> = {
  title: "pages/WorkspacesPage/TemplateMenu",
  component: TemplateMenu,
  args: {
    organizationId: "123",
  },
  parameters: {
    queries: [
      {
        key: getTemplatesQueryKey("123"),
        data: generateTemplates(50),
      },
    ],
  },
};

export default meta;
type Story = StoryObj<typeof TemplateMenu>;

export const Close: Story = {};

export const Open: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /Select template/i });
    await userEvent.click(button);
  },
};

export const Default: Story = {
  args: {
    selected: "2",
  },
};

export const SelectOption: Story = {
  render: function TemplateMenuWithState(args) {
    const [selected, setSelected] = useState<string | undefined>(undefined);
    return (
      <TemplateMenu {...args} selected={selected} onSelect={setSelected} />
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /Select template/i });
    await userEvent.click(button);
    const option = canvas.getByText("Template 4");
    await userEvent.click(option);
  },
};

export const SearchStickyOnTop: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /Select template/i });
    await userEvent.click(button);

    const content = canvasElement.querySelector(".MuiPaper-root");
    content?.scrollTo(0, content.scrollHeight);
  },
};

export const ScrollToSelectedOption: Story = {
  args: {
    selected: "30",
  },

  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /Select template/i });
    await userEvent.click(button);
  },
};

export const Filter: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /Select template/i });
    await userEvent.click(button);
    const filter = canvas.getByLabelText("Search template");
    await userEvent.type(filter, "template23");
  },
};

export const EmptyResults: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /Select template/i });
    await userEvent.click(button);
    const filter = canvas.getByLabelText("Search template");
    await userEvent.type(filter, "invalid-template");
  },
};

export const FocusOnFirstResultWhenPressArrowDown: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /Select template/i });
    await userEvent.click(button);
    const filter = canvas.getByLabelText("Search template");
    await userEvent.type(filter, "template1");
    await userEvent.type(filter, "{arrowdown}");
  },
};

function generateTemplates(amount: number): Partial<Template>[] {
  return Array.from({ length: amount }, (_, i) => ({
    id: i.toString(),
    name: `template${i}`,
    display_name: `Template ${i}`,
    icon: "",
  }));
}
