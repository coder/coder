import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "./Dialog";

const meta: Meta<typeof Dialog> = {
	title: "components/Dialog",
	component: Dialog,
	args: {
		children: (
			<>
				<DialogTrigger asChild>
					<Button>Open Dialog</Button>
				</DialogTrigger>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>Example Dialog Title</DialogTitle>
						<DialogDescription>Dialog Description text</DialogDescription>
					</DialogHeader>
					<DialogFooter>
						<Button>Ok</Button>
					</DialogFooter>
				</DialogContent>
			</>
		),
	},
};

export default meta;
type Story = StoryObj<typeof Dialog>;

export const OpenDialog: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const trigger = canvas.getByRole("button", { name: "Open Dialog" });

		await userEvent.click(trigger);
		await expect(body.getByRole("dialog")).toHaveAttribute(
			"data-state",
			"open",
		);

		await userEvent.keyboard("{Escape}");
		await waitFor(() => {
			expect(body.queryByRole("dialog")).not.toBeInTheDocument();
		});

		await userEvent.click(trigger);
		await expect(body.getByRole("dialog")).toHaveAttribute(
			"data-state",
			"open",
		);
	},
};
