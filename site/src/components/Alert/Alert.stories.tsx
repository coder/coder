import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "components/Button/Button";
import { Alert } from "./Alert";

const meta: Meta<typeof Alert> = {
	title: "components/Alert",
	component: Alert,
};

export default meta;
type Story = StoryObj<typeof Alert>;

const ExampleAction = (
	<Button onClick={() => null} size="sm" variant="subtle">
		Button
	</Button>
);

export const Success: Story = {
	args: {
		children: "You're doing great!",
		severity: "success",
	},
};

export const Warning: Story = {
	args: {
		children: "This is a warning",
		severity: "warning",
	},
};

export const WarningWithDismiss: Story = {
	args: {
		children: "This is a warning",
		dismissible: true,
		severity: "warning",
	},
};

export const WarningWithAction: Story = {
	args: {
		children: "This is a warning",
		actions: [ExampleAction],
		severity: "warning",
	},
};

export const WarningWithActionAndDismiss: Story = {
	args: {
		children: "This is a warning",
		actions: [ExampleAction],
		dismissible: true,
		severity: "warning",
	},
};

export const Info: Story = {
	args: {
		children: "This is an informational message",
		severity: "info",
	},
};

export const ErrorSeverity: Story = {
	args: {
		children: "This is an error message",
		severity: "error",
	},
};

export const WarningProminent: Story = {
	args: {
		children:
			"This is a high risk warning. Use this design only for high risk warnings.",
		severity: "warning",
		prominent: true,
		dismissible: true,
	},
};

export const ErrorProminent: Story = {
	args: {
		children:
			"This is a crucial error. Use this design only for crucial errors.",
		severity: "error",
		prominent: true,
		dismissible: true,
	},
};
