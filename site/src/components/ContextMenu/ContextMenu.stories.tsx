import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import {
	ContextMenu,
	ContextMenuContent,
	ContextMenuItem,
	ContextMenuSeparator,
	ContextMenuSub,
	ContextMenuSubContent,
	ContextMenuSubTrigger,
	ContextMenuTrigger,
} from "./ContextMenu";

const meta: Meta<typeof ContextMenu> = {
	title: "components/ContextMenu",
	component: ContextMenu,
	render: (args) => (
		<ContextMenu {...args}>
			<ContextMenuTrigger className="flex h-40 w-72 items-center justify-center rounded-md border border-dashed border-border">
				Right click here
			</ContextMenuTrigger>
			<ContextMenuContent>
				<ContextMenuItem>Open</ContextMenuItem>
				<ContextMenuItem disabled>Delete</ContextMenuItem>
				<ContextMenuSeparator />
				<ContextMenuSub>
					<ContextMenuSubTrigger>Share</ContextMenuSubTrigger>
					<ContextMenuSubContent>
						<ContextMenuItem>Copy link</ContextMenuItem>
					</ContextMenuSubContent>
				</ContextMenuSub>
			</ContextMenuContent>
		</ContextMenu>
	),
};
export default meta;
type Story = StoryObj<typeof meta>;
export const Closed: Story = {};
export const Open: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.pointer({
			target: within(canvasElement).getByText("Right click here"),
			keys: "[MouseRight]",
		});
	},
};
export const Submenu: Story = {
	play: async (context) => {
		await Open.play?.(context);
		await userEvent.hover(
			await screen.findByRole("menuitem", { name: "Share" }),
		);
		await screen.findByRole("menuitem", { name: "Copy link" });
	},
};
