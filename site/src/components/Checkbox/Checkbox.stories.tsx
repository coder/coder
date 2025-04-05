import type { Meta, StoryObj } from "@storybook/react";
import React from "react";
import { Checkbox } from "./Checkbox";

const meta: Meta<typeof Checkbox> = {
	title: "components/Checkbox",
	component: Checkbox,
	args: {},
	argTypes: {
		checked: {
			control: "boolean",
			description: "The controlled checked state of the checkbox",
		},
		defaultChecked: {
			control: "boolean",
			description: "The default checked state when initially rendered",
		},
		disabled: {
			control: "boolean",
			description:
				"When true, prevents the user from interacting with the checkbox",
		},
	},
};

export default meta;
type Story = StoryObj<typeof Checkbox>;

export const Unchecked: Story = {};

export const Checked: Story = {
	args: {
		defaultChecked: true,
		checked: true,
	},
};

export const Disabled: Story = {
	args: {
		disabled: true,
	},
};

export const DisabledChecked: Story = {
	args: {
		disabled: true,
		defaultChecked: true,
		checked: true,
	},
};

export const CustomStyling: Story = {
	args: {
		className: "h-6 w-6 rounded-full",
	},
};

export const WithLabel: Story = {
	render: () => (
		<div className="flex gap-3">
			<Checkbox id="terms" />
			<div className="grid">
				<label
					htmlFor="terms"
					className="text-sm font-medium peer-disabled:cursor-not-allowed peer-disabled:text-content-disabled"
				>
					Accept terms and conditions
				</label>
				<p className="text-sm text-content-secondary mt-1">
					You agree to our Terms of Service and Privacy Policy.
				</p>
			</div>
		</div>
	),
};

export const Indeterminate: Story = {
	render: () => {
		const [checked, setChecked] = React.useState<boolean | "indeterminate">(
			"indeterminate",
		);
		return (
			<Checkbox
				checked={checked}
				onCheckedChange={(value) => setChecked(value)}
			/>
		);
	},
};
