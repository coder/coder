import type { Meta, StoryObj } from "@storybook/react";
import { userEvent } from "@storybook/test";
import {
	Select,
	SelectContent,
	SelectGroup,
	SelectItem,
	SelectLabel,
	SelectTrigger,
	SelectValue,
} from "./Select";

const meta: Meta<typeof Select> = {
	title: "components/Select",
	component: Select,
	args: {
		children: (
			<>
				<SelectTrigger className="w-[180px]">
					<SelectValue placeholder="Select a fruit" />
				</SelectTrigger>
				<SelectContent>
					<SelectGroup>
						<SelectLabel>Fruits</SelectLabel>
						<SelectItem value="apple">Apple</SelectItem>
						<SelectItem value="banana">Banana</SelectItem>
						<SelectItem value="blueberry">Blueberry</SelectItem>
						<SelectItem value="grapes">Grapes</SelectItem>
						<SelectItem value="pineapple">Pineapple</SelectItem>
					</SelectGroup>
				</SelectContent>
			</>
		),
	},
};

export default meta;
type Story = StoryObj<typeof Select>;

export const Close: Story = {};

export const Open: Story = {
	args: {
		open: true,
	},
};

export const SelectedClose: Story = {
	args: {
		value: "apple",
	},
};

export const SelectedOpen: Story = {
	args: {
		value: "apple",
		open: true,
	},
};
