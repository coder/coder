import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, within, expect } from "@storybook/test";
import { useState } from "react";
import { NewFilter } from "./NewFilter";

const searchLabel = "Search for something";

const meta: Meta<typeof NewFilter> = {
  title: "components/NewFilter",
  component: NewFilter,
  args: {
    id: "search",
    label: searchLabel,
  },
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

export const Error: Story = {
  args: {
    value: "number_of_users:7",
    error: `"number_of_users" is not a valid query param`,
  },
};

export const Focused: Story = {
  args: {
    value: "",
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const input = canvas.getByLabelText(searchLabel);
    await userEvent.click(input);
    await expect(input).toHaveFocus();
  },
};

export const Typing: Story = {
  args: {
    value: "",
  },
  play: async ({ canvasElement }) => {
    const text = "owner:SomeSearchString";
    const canvas = within(canvasElement);
    const input = canvas.getByLabelText(searchLabel);
    await userEvent.type(input, text);
    await expect(input).toHaveValue(text);
  },
};

export const ClearInput: Story = {
  args: {
    value: "owner:CoderUser",
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const input = canvas.getByLabelText(searchLabel);
    await userEvent.click(canvas.getByRole("button", { name: "Clear filter" }));
    await expect(input).toHaveValue("");
  },
};
