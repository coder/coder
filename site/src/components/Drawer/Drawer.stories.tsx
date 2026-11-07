import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
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
				<DrawerTrigger render={<Button />}>Open Drawer</DrawerTrigger>
				<DrawerContent>
					<DrawerHeader>
						<DrawerTitle>Example Drawer Title</DrawerTitle>
						<DrawerDescription>Drawer description text</DrawerDescription>
					</DrawerHeader>
					<DrawerFooter>
						<DrawerClose render={<Button variant="outline" />}>
							Cancel
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
	},
};

export const OpenLeft: Story = {
	args: {
		direction: "left",
		children: (
			<>
				<DrawerTrigger render={<Button />}>Open Left Drawer</DrawerTrigger>
				<DrawerContent>
					<DrawerHeader>
						<DrawerTitle>Left-side drawer</DrawerTitle>
						<DrawerDescription>
							Drawers can slide in from any edge via the direction prop.
						</DrawerDescription>
					</DrawerHeader>
					<DrawerFooter>
						<DrawerClose render={<Button variant="outline" />}>
							Close
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
	},
};

export const CloseWithButton: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Open Drawer" }));
		await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
	},
};
