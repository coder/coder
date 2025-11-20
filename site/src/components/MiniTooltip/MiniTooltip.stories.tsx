import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "components/Button/Button";
import { expect, screen, userEvent, waitFor } from "storybook/test";
import MiniTooltip from "./MiniTooltip";

const title = "Add to library";
const triggerText = "Hover";

const meta: Meta<typeof MiniTooltip> = {
	title: "components/MiniTooltip",
	component: MiniTooltip,
	args: {
		title,
		children: <Button variant="outline">{triggerText}</Button>,
	},
	play: async ({ step }) => {
		await step("activate hover trigger", async () => {
			await userEvent.hover(screen.getByText(triggerText));
			await waitFor(() =>
				expect(screen.getByRole("tooltip")).toHaveTextContent(title),
			);
		});
	},
};

export default meta;
type Story = StoryObj<typeof MiniTooltip>;

export const Default: Story = {};

export const Arrow: Story = { args: { arrow: true } };
