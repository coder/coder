import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, within } from "@storybook/test";
import { useState } from "react";
import { SearchField } from "./SearchField";

const meta: Meta<typeof SearchField> = {
	title: "components/SearchField",
	component: SearchField,
	args: {
		placeholder: "Search...",
	},
	render: function StatefulWrapper(args) {
		const [value, setValue] = useState(args.value);
		return <SearchField {...args} value={value} onChange={setValue} />;
	},
};

export default meta;
type Story = StoryObj<typeof SearchField>;

export const Empty: Story = {};

export const Focused: Story = {
	args: {
		autoFocus: true,
	},
};

export const DefaultValue: Story = {
	args: {
		value: "owner:me",
	},
};

export const TypeValue: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByRole("textbox");
		await userEvent.type(input, "owner:me");
	},
};

export const ClearValue: Story = {
	args: {
		value: "owner:me",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Clear search" }));
	},
};
