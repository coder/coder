import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { Button } from "#/components/Button/Button";
import {
	Drawer,
	DrawerClose,
	DrawerContent,
	DrawerDescription,
	DrawerFooter,
	DrawerHeader,
	DrawerTitle,
	DrawerTrigger,
} from "./Drawer";

const meta: Meta<typeof Drawer> = {
	title: "components/Drawer",
	component: Drawer,
	args: {
		children: (
			<>
				<DrawerTrigger asChild>
					<Button>Open Drawer</Button>
				</DrawerTrigger>
				<DrawerContent>
					<DrawerHeader>
						<DrawerTitle>Example Drawer Title</DrawerTitle>
						<DrawerDescription>Drawer description text</DrawerDescription>
					</DrawerHeader>
					<DrawerFooter>
						<DrawerClose asChild>
							<Button variant="outline">Cancel</Button>
						</DrawerClose>
						<Button>Submit</Button>
					</DrawerFooter>
				</DrawerContent>
			</>
		),
	},
};

export default meta;
type Story = StoryObj<typeof Drawer>;

export const Closed: Story = {};

export const Open: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Open Drawer" }));
		// The drawer renders into a portal on `document.body`, so query the screen.
		await waitFor(() =>
			expect(screen.getByText("Example Drawer Title")).toBeInTheDocument(),
		);
	},
};

export const OpenLeft: Story = {
	args: {
		direction: "left",
		children: (
			<>
				<DrawerTrigger asChild>
					<Button>Open Left Drawer</Button>
				</DrawerTrigger>
				<DrawerContent>
					<DrawerHeader>
						<DrawerTitle>Left-side drawer</DrawerTitle>
						<DrawerDescription>
							Drawers can slide in from any edge via the direction prop.
						</DrawerDescription>
					</DrawerHeader>
					<DrawerFooter>
						<DrawerClose asChild>
							<Button variant="outline">Close</Button>
						</DrawerClose>
					</DrawerFooter>
				</DrawerContent>
			</>
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Open Left Drawer" }),
		);
		await waitFor(() =>
			expect(screen.getByText("Left-side drawer")).toBeInTheDocument(),
		);
	},
};

export const CloseWithButton: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Open Drawer" }));
		await waitFor(() =>
			expect(screen.getByText("Example Drawer Title")).toBeInTheDocument(),
		);
		await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
		await waitFor(() =>
			expect(
				screen.queryByText("Example Drawer Title"),
			).not.toBeInTheDocument(),
		);
	},
};
