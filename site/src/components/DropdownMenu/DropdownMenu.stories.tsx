import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
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
				<DropdownMenuTrigger asChild>
					<Button variant="outline">Admin Settings</Button>
				</DropdownMenuTrigger>
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
		const body = canvasElement.ownerDocument.body;
		const menuItem = await within(body).findByText("Audit logs");
		await user.hover(menuItem);
	},
};

export const NarrowViewport: Story = {
	args: {
		children: (
			<>
				<DropdownMenuTrigger asChild>
					<Button variant="outline">Filters</Button>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="start">
					{Array.from({ length: 24 }, (_, i) => (
						<DropdownMenuItem key={i}>Filter preset {i + 1}</DropdownMenuItem>
					))}
				</DropdownMenuContent>
			</>
		),
	},
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		pixel: { matrix: { viewports: ["phone"] } },
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		await user.click(within(canvasElement).getByText("Filters"));
	},
};
