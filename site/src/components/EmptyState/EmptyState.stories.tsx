import type { Meta, StoryObj } from "@storybook/react";
import { Button } from "components/Button/Button";
import { EmptyState } from "./EmptyState";

const meta: Meta<typeof EmptyState> = {
	title: "components/EmptyState",
	component: EmptyState,
	args: {
		message: "Create your first workspace",
	},
};

export default meta;
type Story = StoryObj<typeof EmptyState>;

export const Example: Story = {
	args: {
		description: "It is easy, just click the button below",
		cta: <Button>Create workspace</Button>,
	},
};

export const Compact: Story = {
	args: {
		description: "It is easy, just click the button below",
		cta: <Button>Create workspace</Button>,
		isCompact: true,
	},
};
