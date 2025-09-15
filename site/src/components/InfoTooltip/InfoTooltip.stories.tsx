import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor } from "storybook/test";
import { InfoTooltip } from "./InfoTooltip";

const meta = {
	title: "components/InfoTooltip",
	component: InfoTooltip,
	args: {
		type: "info",
		title: "Hello, friend!",
		message: "Today is a lovely day :^)",
	},
} satisfies Meta<typeof InfoTooltip>;

export default meta;
type Story = StoryObj<typeof InfoTooltip>;

export const Example: Story = {
	play: async ({ step }) => {
		await step("activate hover trigger", async () => {
			await userEvent.hover(screen.getByRole("button"));
			await waitFor(() =>
				expect(screen.getByRole("tooltip")).toHaveTextContent(
					meta.args.message,
				),
			);
		});
	},
};

export const Notice = {
	args: {
		type: "notice",
		message: "Unfortunately, there's a radio connected to my brain",
	},
	play: async ({ step }) => {
		await step("activate hover trigger", async () => {
			await userEvent.hover(screen.getByRole("button"));
			await waitFor(() =>
				expect(screen.getByRole("tooltip")).toHaveTextContent(
					Notice.args.message,
				),
			);
		});
	},
} satisfies Story;

export const Warning = {
	args: {
		type: "warning",
		message: "Unfortunately, there's a radio connected to my brain",
	},
	play: async ({ step }) => {
		await step("activate hover trigger", async () => {
			await userEvent.hover(screen.getByRole("button"));
			await waitFor(() =>
				expect(screen.getByRole("tooltip")).toHaveTextContent(
					Warning.args.message,
				),
			);
		});
	},
} satisfies Story;
