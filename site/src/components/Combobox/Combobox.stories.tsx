import type { Meta, StoryObj } from "@storybook/react-vite";
import { Combobox } from "./Combobox";

const meta: Meta<typeof Combobox> = {
	title: "components/Combobox",
	component: Combobox,
};

export default meta;

type Story = StoryObj<typeof Combobox>;

export const Default: Story = {
	args: {},
};
