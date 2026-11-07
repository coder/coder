import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "./DropdownMenu";

const meta: Meta<typeof DropdownMenu> = {
	title: "components/DropdownMenu",
	component: DropdownMenu,
	args: {
		children: (
			<>
				<DropdownMenuTrigger
					render={<Button variant="outline">Admin Settings</Button>}
				></DropdownMenuTrigger>
				<DropdownMenuContent>
					<DropdownMenuItem>Deployment</DropdownMenuItem>
					<DropdownMenuItem>Organizations</DropdownMenuItem>
					<DropdownMenuItem>Audit logs</DropdownMenuItem>
					<DropdownMenuItem>Health check</DropdownMenuItem>
				</DropdownMenuContent>
			</>
		),
	},
};

export default meta;
type Story = StoryObj<typeof DropdownMenu>;

export const Close: Story = {};

export const OpenWithHover: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const button = canvas.getByText("Admin Settings");
		await user.click(button);
		const menuItem = await screen.findByRole("menuitem", {
			name: "Audit logs",
		});
		await user.hover(menuItem);
	},
};

export const ScrollablePage: Story = {
	decorators: [
		(Story) => (
			<div className="h-[200vh]">
				<Story />
			</div>
		),
	],
	play: OpenWithHover.play,
};
