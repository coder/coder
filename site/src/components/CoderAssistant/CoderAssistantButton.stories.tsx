import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import { CoderAssistantButton } from "./CoderAssistantButton";

const meta: Meta<typeof CoderAssistantButton> = {
	title: "components/CoderAssistantButton",
	component: CoderAssistantButton,
	// The button is fixed to the bottom-right of the viewport, so
	// fullscreen layout keeps it visible in the canvas.
	parameters: {
		layout: "fullscreen",
	},
	args: {
		open: false,
		onToggle: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof CoderAssistantButton>;

export const Closed: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button", {
			name: "Open Coder Assistant",
		});
		expect(button).toHaveAttribute("aria-expanded", "false");
	},
};

export const Open: Story = {
	args: {
		open: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button", {
			name: "Close Coder Assistant",
		});
		expect(button).toHaveAttribute("aria-expanded", "true");
	},
};

// The thinking and unread indicator dots are purely visual (no roles
// or labels), so these stories are presentational without play
// assertions.
export const Thinking: Story = {
	args: {
		isThinking: true,
	},
};

export const Unread: Story = {
	args: {
		hasUnread: true,
	},
};
