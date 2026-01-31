import type { Meta, StoryObj } from "@storybook/react-vite";
import { SearchIcon } from "lucide-react";
import { InputGroup, InputGroupAddon, InputGroupInput } from "./InputGroup";

const meta: Meta<typeof InputGroup> = {
	title: "components/InputGroup",
	component: InputGroup,
};

export default meta;
type Story = StoryObj<typeof InputGroup>;

export const WithIconStart: Story = {
	args: {
		children: (
			<>
				<InputGroupAddon>
					<SearchIcon />
				</InputGroupAddon>
				<InputGroupInput placeholder="Search..." />
			</>
		),
	},
};

export const WithIconEnd: Story = {
	args: {
		children: (
			<>
				<InputGroupInput placeholder="Search..." />
				<InputGroupAddon align="inline-end">
					<SearchIcon />
				</InputGroupAddon>
			</>
		),
	},
};

export const WithTextStart: Story = {
	args: {
		children: (
			<>
				<InputGroupAddon>https://</InputGroupAddon>
				<InputGroupInput placeholder="example.com" />
			</>
		),
	},
};

export const WithTextEnd: Story = {
	args: {
		children: (
			<>
				<InputGroupInput placeholder="username" />
				<InputGroupAddon align="inline-end">@coder.com</InputGroupAddon>
			</>
		),
	},
};

export const WithBothAddons: Story = {
	args: {
		children: (
			<>
				<InputGroupAddon>$</InputGroupAddon>
				<InputGroupInput placeholder="0.00" type="number" />
				<InputGroupAddon align="inline-end">USD</InputGroupAddon>
			</>
		),
	},
};

export const Disabled: Story = {
	args: {
		children: (
			<>
				<InputGroupAddon>
					<SearchIcon />
				</InputGroupAddon>
				<InputGroupInput placeholder="Disabled..." disabled />
			</>
		),
	},
};

export const Invalid: Story = {
	args: {
		children: (
			<>
				<InputGroupAddon>
					<SearchIcon />
				</InputGroupAddon>
				<InputGroupInput placeholder="Invalid..." aria-invalid="true" />
			</>
		),
	},
};
