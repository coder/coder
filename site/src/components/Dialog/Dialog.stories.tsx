import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import { Button } from "#/components/Button/Button";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
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
				<DialogTrigger render={<Button />}>Open Dialog</DialogTrigger>
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
		await userEvent.click(canvas.getByRole("button", { name: "Open Dialog" }));
	},
};

export const WithSelect: Story = {
	args: {
		defaultOpen: true,
		children: (
			<DialogContent>
				<DialogTitle>Preferences</DialogTitle>
				<Select defaultValue="apple">
					<SelectTrigger aria-label="Fruit">
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						<SelectItem value="apple">Apple</SelectItem>
						<SelectItem value="banana">Banana</SelectItem>
					</SelectContent>
				</Select>
			</DialogContent>
		),
	},
	play: async () => {
		await userEvent.click(screen.getByRole("combobox", { name: "Fruit" }));
	},
};

export const ScrollablePage: Story = {
	play: OpenDialog.play,
	decorators: [
		(Story) => (
			<div className="h-[200vh]">
				<Story />
			</div>
		),
	],
};
