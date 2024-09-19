import type { Meta, StoryObj } from "@storybook/react";
import { Label } from "./Label";
import ErrorOutline from "@mui/icons-material/ErrorOutline";

const meta: Meta<typeof Label> = {
	title: "components/GanttChart/Label",
	component: Label,
	args: {
		children: "5s",
	},
};

export default meta;
type Story = StoryObj<typeof Label>;

export const Default: Story = {};

export const SecondaryColor: Story = {
	args: {
		color: "secondary",
	},
};

export const StartIcon: Story = {
	args: {
		children: (
			<>
				<ErrorOutline />
				docker_value
			</>
		),
	},
};
