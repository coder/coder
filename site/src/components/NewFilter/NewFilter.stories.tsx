import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, within, expect } from "@storybook/test";
import { useState } from "react";
import { NewFilter } from "./NewFilter";

const meta: Meta<typeof NewFilter> = {
  title: "components/NewFilter",
  component: NewFilter,
  render: function NewFilterWithState(args) {
    const [value, setValue] = useState<string>(args.value);
    return <NewFilter {...args} value={value} onChange={setValue} />;
  },
};

export default meta;
type Story = StoryObj<typeof NewFilter>;

export const Empty: Story = {
  args: {
    value: "",
  },
};

export const DefaultValue: Story = {
  args: {
    value: "owner:CoderUser",
  },
};

export const Focused: Story = {
  args: {
    value: "",
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("textbox"));
    await expect(canvas.getByRole("textbox")).toHaveFocus();
  },
};

export const Typing: Story = {
  args: {
    value: "",
  },
  play: async ({ canvasElement }) => {
    const text = "owner:SomeSearchString";
    const canvas = within(canvasElement);
    await userEvent.type(canvas.getByRole("textbox"), text);
    await expect(canvas.getByRole("textbox")).toHaveValue(text);
  },
};

export const ClearInput: Story = {
  args: {
    value: "owner:CoderUser",
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Clear filter" }));
    await expect(canvas.getByRole("textbox")).toHaveValue("");
  },
};
