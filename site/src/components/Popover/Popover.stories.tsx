import type { Meta, StoryObj } from "@storybook/react";
import { expect, screen, userEvent, waitFor, within } from "@storybook/test";
import { Button } from "components/Button/Button";
import { Popover, PopoverContent, PopoverTrigger } from "./Popover";

const meta: Meta<typeof Popover> = {
	title: "components/Popover",
	component: Popover,
};

export default meta;
type Story = StoryObj<typeof Popover>;

const content = `
According to all known laws of aviation, there is no way a bee should be able to fly.
Its wings are too small to get its fat little body off the ground. The bee, of course,
flies anyway because bees don't care what humans think is impossible.
`;

export const Default: Story = {
	args: {
		children: (
			<Popover>
				<PopoverTrigger asChild>
					<Button className="ml-20">Click here!</Button>
				</PopoverTrigger>
				<PopoverContent>{content}</PopoverContent>
			</Popover>
		),
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("click to open", async () => {
			await userEvent.click(canvas.getByRole("button"));
			await waitFor(() =>
				expect(
					screen.getByText(/according to all known laws/i),
				).toBeInTheDocument(),
			);
		});
	},
};

export const AlignStart: Story = {
	args: {
		children: (
			<Popover>
				<PopoverTrigger asChild>
					<Button className="ml-20">Click here!</Button>
				</PopoverTrigger>
				<PopoverContent align="start">{content}</PopoverContent>
			</Popover>
		),
	},
	play: Default.play,
};
