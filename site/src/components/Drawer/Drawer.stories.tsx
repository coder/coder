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
				<DrawerTrigger render={<Button>Open Drawer</Button>} />
				<DrawerContent>
					<DrawerHeader>
						<DrawerTitle>Example Drawer Title</DrawerTitle>
						<DrawerDescription>Drawer description text</DrawerDescription>
					</DrawerHeader>
					<DrawerFooter>
						<DrawerClose render={<Button variant="outline">Cancel</Button>} />
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
		await expect(await screen.findByRole("dialog")).toBeInTheDocument();
	},
};

export const CloseDrawer: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Open Drawer" }));
		const dialog = await screen.findByRole("dialog");
		await userEvent.click(
			within(dialog).getByRole("button", { name: "Cancel" }),
		);
		await waitFor(() =>
			expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
		);
	},
};

export const OpenRightDrawer: Story = {
	args: {
		direction: "right",
		children: (
			<>
				<DrawerTrigger render={<Button>Open Right Drawer</Button>} />
				<DrawerContent className="sm:max-w-md">
					<DrawerHeader>
						<DrawerTitle>Right-side drawer</DrawerTitle>
						<DrawerDescription>
							Drawers can open from any side via the direction prop.
						</DrawerDescription>
					</DrawerHeader>
					<DrawerFooter>
						<DrawerClose render={<Button variant="outline">Close</Button>} />
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
		await expect(await screen.findByRole("dialog")).toBeInTheDocument();
	},
};
