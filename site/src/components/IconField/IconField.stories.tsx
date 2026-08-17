import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
import { IconField } from "./IconField";

const meta: Meta<typeof IconField> = {
	title: "components/IconField",
	component: IconField,
	args: {
		onPickEmoji: fn(),
		onChange: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof IconField>;

export const Example: Story = {};

export const EmojiSelected: Story = {
	args: {
		value: "/emojis/1f3f3-fe0f-200d-26a7-fe0f.png",
	},
};

export const IconSelected: Story = {
	args: {
		value: "/icon/fedora.svg",
	},
};

export const WithHelperText: Story = {
	args: {
		helperText: "Paste an image URL or pick an emoji.",
	},
};

export const WithError: Story = {
	args: {
		error: true,
		helperText: "Icon URL is too long.",
		value: "https://example.com/very-long-icon-url.png",
	},
};

export const OpenPicker: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button", {
			name: "Pick an emoji or icon",
		});
		await userEvent.click(button);
		await expect(button).toHaveAttribute("aria-expanded", "true");
		const popover = await screen.findByRole("dialog");
		await waitFor(() =>
			expect(popover.querySelector("em-emoji-picker")).toBeInTheDocument(),
		);
	},
};
