import type { Meta, StoryObj } from "@storybook/react";
import { PlusIcon } from "lucide-react";
import { Spinner } from "./Spinner";

const meta: Meta<typeof Spinner> = {
	title: "components/Spinner",
	component: Spinner,
	args: {
		children: <PlusIcon className="size-icon-lg" />,
	},
};

export default meta;
type Story = StoryObj<typeof Spinner>;

export const Idle: Story = {};

export const Loading: Story = {
	args: { loading: true },
};
