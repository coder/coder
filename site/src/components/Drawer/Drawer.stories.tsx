import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
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

export const OpenDrawer: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Open Drawer" }));
	},
};

export const OpenRightDrawer: Story = {
	args: {
		direction: "right",
		children: (
			<>
				<DrawerTrigger asChild>
					<Button>Open Right Drawer</Button>
				</DrawerTrigger>
				<DrawerContent className="sm:max-w-md">
					<DrawerHeader>
						<DrawerTitle>Right-side drawer</DrawerTitle>
						<DrawerDescription>
							Drawers can open from any side via the direction prop.
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
			canvas.getByRole("button", { name: "Open Right Drawer" }),
		);
	},
};
